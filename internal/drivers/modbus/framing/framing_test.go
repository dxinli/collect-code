package framing_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/dxinli/collect-code/internal/drivers/modbus/framing"
	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
)

func TestFramerSwitchSharesPDU(t *testing.T) {
	rawPDU := []byte{0x03, 0x00, 0x00, 0x00, 0x02}
	for _, framer := range []framing.Framer{framing.NewMBAP(), framing.NewRTU()} {
		encoded, err := framer.Encode(framing.Frame{TransactionID: 1, UnitID: 1, PDU: rawPDU})
		if err != nil {
			t.Fatalf("%s Encode: %v", framer.Name(), err)
		}
		frame, err := framer.DecodeRequest(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("%s DecodeRequest: %v", framer.Name(), err)
		}
		if !bytes.Equal(frame.PDU, rawPDU) {
			t.Fatalf("%s PDU = % X, want % X", framer.Name(), frame.PDU, rawPDU)
		}
	}
}

func TestFramerPDURecoverability(t *testing.T) {
	if !framing.NewMBAP().PDUErrorsAreRecoverable() {
		t.Fatal("MBAP PDU errors must be recoverable")
	}
	if framing.NewRTU().PDUErrorsAreRecoverable() {
		t.Fatal("RTU PDU errors must be unrecoverable")
	}
}

func TestRTUCleanEOFBeforeFrame(t *testing.T) {
	for _, framer := range []framing.Framer{framing.NewRTU(), framing.NewMBAP()} {
		_, err := framer.DecodeRequest(bytes.NewReader(nil))
		if !errors.Is(err, io.EOF) {
			t.Fatalf("%s empty err = %v, want io.EOF", framer.Name(), err)
		}
	}
}

func TestFramingErrorsSatisfyProtocol(t *testing.T) {
	errs := []error{
		framing.ErrBadCRC,
		framing.ErrShortFrame,
		framing.ErrFrameTooLong,
		framing.ErrMalformedFrame,
		framing.ErrTransactionMismatch,
	}
	for _, err := range errs {
		if !errors.Is(err, pdu.ErrProtocol) {
			t.Fatalf("%v does not satisfy pdu.ErrProtocol", err)
		}
	}
}
