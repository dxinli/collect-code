package sim

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultModbusAddr    = "127.0.0.1:0"
	defaultModbusRTUAddr = "127.0.0.1:0"

	pathModbusRequests = "/_sim/requests"
)

// ModbusOptions configures the simulated Modbus slave.
type ModbusOptions struct {
	TCPAddr   string
	RTUAddr   string
	ConfigDir string
	Now       func() time.Time
	Logger    *slog.Logger
}

// modbusSession bundles the connection metadata threaded through request
// handling.
type modbusSession struct {
	conn      net.Conn
	transport string
	remote    string
}

// Modbus is the in-process Modbus TCP / RTU-over-TCP slave. The two listeners
// share one unit table, request log and control state.
type Modbus struct {
	mu sync.Mutex

	tcpLn   net.Listener
	rtuLn   net.Listener
	tcpHost string
	tcpPort int
	rtuHost string
	rtuPort int

	units          map[uint8]*unitState
	unitIDs        []int
	registerSource string
	requests       *modbusRequestLog

	now    func() time.Time
	logger *slog.Logger

	started   bool
	closed    bool
	closeOnce sync.Once
	done      chan struct{}
	wg        sync.WaitGroup
	conns     map[net.Conn]struct{}
}

// NewModbus builds the slave and binds both TCP listeners. Call Register and
// Start before serving.
func NewModbus(opts ModbusOptions) (*Modbus, error) {
	tcpAddr := strings.TrimSpace(opts.TCPAddr)
	if tcpAddr == "" {
		tcpAddr = defaultModbusAddr
	}
	rtuAddr := strings.TrimSpace(opts.RTUAddr)
	if rtuAddr == "" {
		rtuAddr = defaultModbusRTUAddr
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}

	units := make(map[uint8]*unitState)
	source := "builtin"
	configDir := strings.TrimSpace(opts.ConfigDir)
	if configDir != "" {
		var err error
		units, err = loadModbusConfig(configDir)
		if err != nil {
			return nil, err
		}
		source = "config"
	} else {
		for _, id := range builtinUnitIDs() {
			units[id] = newBuiltinUnitState()
		}
	}

	tcpLn, err := net.Listen("tcp", tcpAddr)
	if err != nil {
		return nil, fmt.Errorf("sim: listen modbus tcp %s: %w", tcpAddr, err)
	}
	rtuLn, err := net.Listen("tcp", rtuAddr)
	if err != nil {
		_ = tcpLn.Close()
		return nil, fmt.Errorf("sim: listen modbus rtu-over-tcp %s: %w", rtuAddr, err)
	}
	tcpHost, tcpPort, err := splitListenAddr(tcpLn.Addr().String())
	if err != nil {
		_ = tcpLn.Close()
		_ = rtuLn.Close()
		return nil, err
	}
	rtuHost, rtuPort, err := splitListenAddr(rtuLn.Addr().String())
	if err != nil {
		_ = tcpLn.Close()
		_ = rtuLn.Close()
		return nil, err
	}

	return &Modbus{
		tcpLn:          tcpLn,
		rtuLn:          rtuLn,
		tcpHost:        tcpHost,
		tcpPort:        tcpPort,
		rtuHost:        rtuHost,
		rtuPort:        rtuPort,
		units:          units,
		unitIDs:        sortedUnitIDs(units),
		registerSource: source,
		requests:       newModbusRequestLog(modbusRequestLogCapacity),
		now:            opts.Now,
		logger:         logger,
		done:           make(chan struct{}),
		conns:          make(map[net.Conn]struct{}),
	}, nil
}

// Register contributes the modbus manifest section, request-log endpoint and
// control actions to the runtime. It must run before Start.
func (m *Modbus) Register(r *Runtime) error {
	if r == nil {
		return errors.New("sim: runtime is required")
	}
	m.mu.Lock()
	if m.now == nil {
		m.now = r.Now
	}
	m.mu.Unlock()

	if err := r.SetSection("modbus", m.manifestSection()); err != nil {
		return err
	}
	if err := r.SetEndpoint("requests", pathModbusRequests); err != nil {
		return err
	}
	r.Handle("GET "+pathModbusRequests, m.handleRequests)
	for action, fn := range map[string]ControlFunc{
		"unit_offline": m.handleUnitOffline,
		"unit_slow":    m.handleUnitSlow,
		"unit_bad_crc": m.handleUnitBadCRC,
	} {
		if err := r.RegisterControl("modbus", action, fn); err != nil {
			return err
		}
	}
	return nil
}

// Start begins accepting clients on both listeners.
func (m *Modbus) Start() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("sim: modbus is closed")
	}
	if m.started {
		m.mu.Unlock()
		return errors.New("sim: modbus is already started")
	}
	m.started = true
	if m.now == nil {
		m.now = time.Now
	}
	m.mu.Unlock()

	m.wg.Add(2)
	go m.acceptLoop(m.tcpLn, modbusTCPTransport)
	go m.acceptLoop(m.rtuLn, modbusRTUTransport)
	return nil
}

// Close stops both listeners, closes active connections and waits for the
// serving goroutines.
func (m *Modbus) Close() error {
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		close(m.done)
		conns := make([]net.Conn, 0, len(m.conns))
		for conn := range m.conns {
			conns = append(conns, conn)
		}
		m.mu.Unlock()

		if m.tcpLn != nil {
			_ = m.tcpLn.Close()
		}
		if m.rtuLn != nil {
			_ = m.rtuLn.Close()
		}
		for _, conn := range conns {
			_ = conn.Close()
		}
		m.wg.Wait()
	})
	return nil
}

// TCPAddress returns the actual Modbus TCP host:port.
func (m *Modbus) TCPAddress() string {
	return net.JoinHostPort(m.tcpHost, strconv.Itoa(m.tcpPort))
}

// RTUAddress returns the actual RTU-over-TCP host:port.
func (m *Modbus) RTUAddress() string {
	return net.JoinHostPort(m.rtuHost, strconv.Itoa(m.rtuPort))
}

func (m *Modbus) manifestSection() map[string]any {
	return map[string]any{
		"tcp": map[string]any{
			"url":  "tcp://" + m.TCPAddress(),
			"host": m.tcpHost,
			"port": m.tcpPort,
		},
		"rtu_over_tcp": map[string]any{
			"url":  "tcp://" + m.RTUAddress(),
			"host": m.rtuHost,
			"port": m.rtuPort,
		},
		"units":           m.unitIDs,
		"register_source": m.registerSource,
	}
}

func (m *Modbus) acceptLoop(ln net.Listener, transport string) {
	defer m.wg.Done()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || m.isClosed() {
				return
			}
			m.logger.Error("modbus accept failed", "transport", transport, "error", err)
			return
		}
		if !m.trackConn(conn) {
			return
		}
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			defer m.untrackConn(conn)
			m.serveConn(conn, transport)
		}()
	}
}

func (m *Modbus) serveConn(conn net.Conn, transport string) {
	defer func() { _ = conn.Close() }()
	session := &modbusSession{conn: conn, transport: transport, remote: conn.RemoteAddr().String()}
	reader := bufio.NewReader(conn)

	if transport == modbusRTUTransport {
		for {
			adu, _, err := readRTU(reader)
			if err != nil {
				var frameErr *modbusFrameError
				if errors.As(err, &frameErr) {
					m.logFrameBad(session, frameErr)
					if frameErr.fatal {
						return
					}
					continue
				}
				if !m.isClosed() && !errors.Is(err, io.EOF) {
					m.logger.Debug("modbus rtu read stopped", "remote", session.remote, "error", err)
				}
				return
			}
			if !m.handleADU(session, adu) {
				return
			}
		}
	}

	for {
		adu, err := readMBAP(reader)
		if err != nil {
			var frameErr *modbusFrameError
			if errors.As(err, &frameErr) {
				m.logFrameBad(session, frameErr)
			} else if !m.isClosed() && !errors.Is(err, io.EOF) {
				m.logger.Debug("modbus tcp read stopped", "remote", session.remote, "error", err)
			}
			return
		}
		if !m.handleADU(session, adu) {
			return
		}
	}
}

func (m *Modbus) handleADU(session *modbusSession, adu modbusADU) bool {
	started := time.Now()
	received := m.now().UTC()
	entry := modbusRequestEntry{
		Seq:        m.requests.nextSeq(),
		Transport:  session.transport,
		Remote:     session.remote,
		UnitID:     modbusUint8Ptr(adu.unitID),
		ReceivedAt: received,
	}
	if len(adu.pdu) > 0 {
		entry.Function = modbusUint8Ptr(adu.pdu[0])
	}

	req, parseErr := parseModbusRequest(adu.pdu)
	if parseErr == nil {
		entry.Address = modbusUint16Ptr(req.address)
		entry.Quantity = modbusUint16Ptr(req.quantity)
		switch req.function {
		case modbusFuncWriteSingle:
			entry.Values = []uint16{req.value}
		case modbusFuncWriteMultiple:
			entry.Values = append([]uint16(nil), req.values...)
		}
	}

	m.mu.Lock()
	unit, ok := m.units[adu.unitID]
	if !ok {
		m.mu.Unlock()
		entry.Outcome = modbusOutcomeNoResponse
		m.requests.add(entry)
		return true
	}
	fault := m.faultFor(unit)
	if fault.kind == modbusFaultOffline {
		m.mu.Unlock()
		entry.Outcome = modbusOutcomeNoResponse
		m.requests.add(entry)
		return true
	}
	var (
		response  []byte
		exception uint8
	)
	if parseErr != nil {
		var pduErr *modbusPDUError
		if !errors.As(parseErr, &pduErr) || len(adu.pdu) == 0 {
			m.mu.Unlock()
			m.logger.Warn("modbus parse failed", "error", parseErr)
			return false
		}
		response = modbusExceptionResponse(adu.pdu[0], pduErr.code)
		exception = pduErr.code
	} else {
		response, exception = m.dispatch(unit, req)
	}
	m.mu.Unlock()

	if fault.kind == modbusFaultSlow && !m.wait(fault.delay) {
		return false
	}

	switch {
	case fault.kind == modbusFaultBadCRC && session.transport == modbusRTUTransport:
		entry.Outcome = modbusOutcomeBadCRC
		if exception != 0 {
			entry.Exception = modbusUint8Ptr(exception)
		}
	case exception != 0:
		entry.Outcome = modbusOutcomeException
		entry.Exception = modbusUint8Ptr(exception)
	default:
		entry.Outcome = modbusOutcomeOK
	}
	return m.writeAndLog(session, adu, entry, response, fault, started)
}

func (m *Modbus) dispatch(unit *unitState, req modbusRequest) ([]byte, uint8) {
	switch req.function {
	case modbusFuncReadHolding:
		values, code := unit.read(false, req.address, req.quantity)
		if code != 0 {
			return modbusExceptionResponse(req.function, code), code
		}
		return modbusReadResponse(req.function, values), 0
	case modbusFuncReadInput:
		values, code := unit.read(true, req.address, req.quantity)
		if code != 0 {
			return modbusExceptionResponse(req.function, code), code
		}
		return modbusReadResponse(req.function, values), 0
	case modbusFuncWriteSingle:
		code := unit.writeSingle(req.address, req.value)
		if code != 0 {
			return modbusExceptionResponse(req.function, code), code
		}
		return modbusWriteSingleResponse(req.address, req.value), 0
	case modbusFuncWriteMultiple:
		code := unit.writeMultiple(req.address, req.values)
		if code != 0 {
			return modbusExceptionResponse(req.function, code), code
		}
		return modbusWriteMultipleResponse(req.address, req.quantity), 0
	default:
		return modbusExceptionResponse(req.function, modbusExceptionIllegalFunction),
			modbusExceptionIllegalFunction
	}
}

func (m *Modbus) writeAndLog(session *modbusSession, adu modbusADU,
	entry modbusRequestEntry, pdu []byte, fault modbusFault, started time.Time,
) bool {
	var (
		raw []byte
		err error
	)
	if session.transport == modbusTCPTransport {
		raw, err = encodeMBAP(modbusADU{unitID: adu.unitID, pdu: pdu, txnID: adu.txnID})
	} else {
		raw, err = encodeRTU(modbusADU{unitID: adu.unitID, pdu: pdu},
			fault.kind == modbusFaultBadCRC)
	}
	if err != nil {
		m.logger.Error("modbus encode response", "transport", session.transport, "error", err)
		return false
	}
	if err := writeModbusFull(session.conn, raw); err != nil {
		if !m.isClosed() {
			m.logger.Warn("modbus write failed", "transport", session.transport, "remote", session.remote, "error", err)
		}
		return false
	}
	responded := m.now().UTC()
	entry.RespondedAt = &responded
	entry.DurationMS = time.Since(started).Milliseconds()
	if entry.DurationMS < 0 {
		entry.DurationMS = 0
	}
	m.requests.add(entry)
	return true
}

func (m *Modbus) logFrameBad(session *modbusSession, frameErr *modbusFrameError) {
	entry := modbusRequestEntry{
		Seq:        m.requests.nextSeq(),
		Transport:  session.transport,
		Remote:     session.remote,
		Outcome:    modbusOutcomeBadFrame,
		RawHex:     modbusRawHex(frameErr.raw),
		ReceivedAt: m.now().UTC(),
	}
	if len(frameErr.raw) > 0 {
		entry.UnitID = modbusUint8Ptr(frameErr.adu.unitID)
	}
	if len(frameErr.adu.pdu) > 0 {
		entry.Function = modbusUint8Ptr(frameErr.adu.pdu[0])
	}
	m.requests.add(entry)
	m.logger.Warn("modbus bad frame", "transport", session.transport, "remote", session.remote, "error", frameErr.msg)
}

func (m *Modbus) wait(delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-m.done:
		return false
	}
}

func (m *Modbus) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func (m *Modbus) trackConn(conn net.Conn) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		_ = conn.Close()
		return false
	}
	m.conns[conn] = struct{}{}
	return true
}

func (m *Modbus) untrackConn(conn net.Conn) {
	m.mu.Lock()
	delete(m.conns, conn)
	m.mu.Unlock()
}

func splitListenAddr(addr string) (string, int, error) {
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, fmt.Errorf("sim: modbus address %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", 0, fmt.Errorf("sim: modbus port %q: %w", portText, err)
	}
	return host, port, nil
}

func writeModbusFull(conn net.Conn, raw []byte) error {
	for len(raw) > 0 {
		n, err := conn.Write(raw)
		if err != nil {
			return err
		}
		raw = raw[n:]
	}
	return nil
}
