package pdu_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
)

func TestEncodeRequestGolden(t *testing.T) {
	tests := []struct {
		name string
		req  pdu.Request
		want []byte
	}{
		{"read holding", &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Address: 0, Quantity: 2}, []byte{0x03, 0x00, 0x00, 0x00, 0x02}},
		{"read input", &pdu.ReadRegistersRequest{Func: pdu.FuncReadInputRegisters, Address: 0, Quantity: 1}, []byte{0x04, 0x00, 0x00, 0x00, 0x01}},
		{"write single", &pdu.WriteSingleRegisterRequest{Address: 2, Value: 50}, []byte{0x06, 0x00, 0x02, 0x00, 0x32}},
		{"write multiple", &pdu.WriteMultipleRegistersRequest{Address: 2, Values: []uint16{50, 100}}, []byte{0x10, 0x00, 0x02, 0x00, 0x02, 0x04, 0x00, 0x32, 0x00, 0x64}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pdu.EncodeRequest(tt.req)
			if err != nil {
				t.Fatalf("EncodeRequest: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("EncodeRequest = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestDecodeRequestGolden(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want pdu.Request
	}{
		{"read holding", []byte{0x03, 0x00, 0x00, 0x00, 0x02}, &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Address: 0, Quantity: 2}},
		{"read input", []byte{0x04, 0x00, 0x00, 0x00, 0x01}, &pdu.ReadRegistersRequest{Func: pdu.FuncReadInputRegisters, Address: 0, Quantity: 1}},
		{"write single", []byte{0x06, 0x00, 0x02, 0x00, 0x32}, &pdu.WriteSingleRegisterRequest{Address: 2, Value: 50}},
		{"write multiple", []byte{0x10, 0x00, 0x02, 0x00, 0x02, 0x04, 0x00, 0x32, 0x00, 0x64}, &pdu.WriteMultipleRegistersRequest{Address: 2, Values: []uint16{50, 100}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pdu.DecodeRequest(tt.raw)
			if err != nil {
				t.Fatalf("DecodeRequest: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DecodeRequest = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestEncodeResponseGolden(t *testing.T) {
	tests := []struct {
		name string
		resp pdu.Response
		want []byte
	}{
		{"read holding", &pdu.ReadRegistersResponse{Func: pdu.FuncReadHoldingRegisters, Registers: []uint16{0x0032, 0x0064}}, []byte{0x03, 0x04, 0x00, 0x32, 0x00, 0x64}},
		{"read input", &pdu.ReadRegistersResponse{Func: pdu.FuncReadInputRegisters, Registers: []uint16{0xFFFF}}, []byte{0x04, 0x02, 0xFF, 0xFF}},
		{"write single", &pdu.WriteSingleRegisterResponse{Address: 2, Value: 50}, []byte{0x06, 0x00, 0x02, 0x00, 0x32}},
		{"write multiple", &pdu.WriteMultipleRegistersResponse{Address: 2, Quantity: 2}, []byte{0x10, 0x00, 0x02, 0x00, 0x02}},
		{"exception", &pdu.ExceptionResponse{Func: pdu.FuncReadHoldingRegisters, Code: pdu.ExceptionIllegalDataAddress}, []byte{0x83, 0x02}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pdu.EncodeResponse(tt.resp)
			if err != nil {
				t.Fatalf("EncodeResponse: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("EncodeResponse = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestDecodeResponseGolden(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want pdu.Response
	}{
		{"read holding", []byte{0x03, 0x04, 0x00, 0x32, 0x00, 0x64}, &pdu.ReadRegistersResponse{Func: pdu.FuncReadHoldingRegisters, Registers: []uint16{0x0032, 0x0064}}},
		{"read input negative raw", []byte{0x04, 0x02, 0xFF, 0xFF}, &pdu.ReadRegistersResponse{Func: pdu.FuncReadInputRegisters, Registers: []uint16{0xFFFF}}},
		{"write single", []byte{0x06, 0x00, 0x02, 0x00, 0x32}, &pdu.WriteSingleRegisterResponse{Address: 2, Value: 50}},
		{"write multiple", []byte{0x10, 0x00, 0x02, 0x00, 0x02}, &pdu.WriteMultipleRegistersResponse{Address: 2, Quantity: 2}},
		{"exception illegal address", []byte{0x83, 0x02}, &pdu.ExceptionResponse{Func: pdu.FuncReadHoldingRegisters, Code: pdu.ExceptionIllegalDataAddress}},
		{"exception device failure", []byte{0x84, 0x04}, &pdu.ExceptionResponse{Func: pdu.FuncReadInputRegisters, Code: pdu.ExceptionServerDeviceFailure}},
		{"exception reserved zero", []byte{0x83, 0x00}, &pdu.ExceptionResponse{Func: pdu.FuncReadHoldingRegisters, Code: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pdu.DecodeResponse(tt.raw)
			if err != nil {
				t.Fatalf("DecodeResponse: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DecodeResponse = %#v, want %#v", got, tt.want)
			}
		})
	}
}
