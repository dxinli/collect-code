package framing

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
)

type mbap struct{}

// NewMBAP returns the Modbus TCP framing.
func NewMBAP() Framer { return mbap{} }

func (mbap) Name() string { return "mbap" }

func (mbap) PDUErrorsAreRecoverable() bool { return true }

func (mbap) Encode(f Frame) ([]byte, error) {
	if err := validateFrame(f); err != nil {
		return nil, err
	}
	raw := make([]byte, 7+len(f.PDU))
	binary.BigEndian.PutUint16(raw[0:2], f.TransactionID)
	binary.BigEndian.PutUint16(raw[2:4], 0)
	binary.BigEndian.PutUint16(raw[4:6], uint16(len(f.PDU)+1))
	raw[6] = f.UnitID
	copy(raw[7:], f.PDU)
	return raw, nil
}

func (mbap) DecodeRequest(r io.Reader) (Frame, error) {
	return decodeMBAP(r, 0, false)
}

func (mbap) DecodeResponse(r io.Reader, _ pdu.Request, expectTxn uint16) (Frame, error) {
	return decodeMBAP(r, expectTxn, true)
}

func decodeMBAP(r io.Reader, expectTxn uint16, checkTxn bool) (Frame, error) {
	var header [7]byte
	if err := readFull(r, header[:]); err != nil {
		return Frame{}, err
	}
	txn := binary.BigEndian.Uint16(header[0:2])
	proto := binary.BigEndian.Uint16(header[2:4])
	length := binary.BigEndian.Uint16(header[4:6])
	unitID := header[6]
	if length < 2 {
		return Frame{}, unrecoverable(fmt.Errorf("%w: MBAP length %d", ErrMalformedFrame, length))
	}
	if length > 254 {
		return Frame{}, unrecoverable(fmt.Errorf("%w: MBAP length %d", ErrFrameTooLong, length))
	}
	body := make([]byte, int(length)-1)
	if err := readFull(r, body); err != nil {
		return Frame{}, bodyReadError(err)
	}
	if proto != 0 {
		return Frame{}, fmt.Errorf("%w: MBAP protocol id %d", ErrMalformedFrame, proto)
	}
	if checkTxn && txn != expectTxn {
		return Frame{}, fmt.Errorf("%w: got %d want %d", ErrTransactionMismatch, txn, expectTxn)
	}
	return Frame{TransactionID: txn, UnitID: unitID, PDU: body}, nil
}

func bodyReadError(err error) error {
	return unrecoverable(errors.Join(ErrShortFrame, err))
}
