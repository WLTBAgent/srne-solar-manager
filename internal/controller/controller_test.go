package controller

import (
	"strings"
	"testing"
)

func reg(addr uint16, length int, scale float64, signed bool) *Register {
	return &Register{Address: addr, Length: length, Scale: scale, Signed: signed}
}

func TestDecodeValueScaled(t *testing.T) {
	cases := []struct {
		name string
		reg  *Register
		raw  []uint16
		want string
	}{
		{"battery voltage", &Register{Address: 0x0101, Scale: 0.1, Unit: "V"}, []uint16{485}, "48.5 V"},
		{"load current 2 decimals", &Register{Address: 0x0105, Scale: 0.01, Unit: "A"}, []uint16{1234}, "12.34 A"},
		{"plain watts", &Register{Address: 0x0109, Unit: "W"}, []uint16{1200}, "1200 W"},
		{"signed temp", &Register{Address: 0x0223, Scale: 0.1, Signed: true, Unit: "°C"}, []uint16{65518}, "-1.8 °C"},
		{"charging 30 A (device 0xFED4)", FindRegisterByAddr(0x0102), []uint16{0xFED4}, "30.0 A"},
		{"device -8.5 (charging) displays positive", FindRegisterByAddr(0x0102), []uint16{0xFFAB}, "8.5 A"},
		{"device +8.5 (discharging) displays negative", FindRegisterByAddr(0x0102), []uint16{85}, "-8.5 A"},
		{"n/a marker single word", &Register{Address: 0x0108, Scale: 0.1, Unit: "A"}, []uint16{0xFFFF}, "n/a"},
		{"n/a marker multi word", &Register{Address: 0xF034, Length: 2, Category: "Statistics", Unit: "AH"}, []uint16{0xFFFF, 0xFFFF}, "n/a"},
		{"enum", &Register{Address: 0xE004, Enum: map[uint16]string{3: "Default"}}, []uint16{3}, "Default"},
		{"enum fallback to number", &Register{Address: 0xE004}, []uint16{9}, "9"},
		{"empty raw", &Register{Address: 1}, nil, "---"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecodeValue(tc.reg, tc.raw); got != tc.want {
				t.Errorf("DecodeValue() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeSpecialRegisters(t *testing.T) {
	model := encodeLowByteString("HF4850S80-100")
	cases := []struct {
		name string
		reg  *Register
		raw  []uint16
		want string
	}{
		{"product model ASCII", &Register{Address: addrProductModel, Length: 8}, model, "HF4850S80-100"},
		{"software version", &Register{Address: addrSoftwareVersion, Length: 2}, []uint16{100, 105}, "CPU1 V1.00 · CPU2 V1.05"},
		{"rated sys", &Register{Address: addrRatedSys, Length: 1}, []uint16{0x3042}, "48 V system · rated 66 A"},
		{"rated sys auto", &Register{Address: addrRatedSys, Length: 1}, []uint16{0xFF2D}, "Auto system voltage · rated 45 A"},
		{"device temp", &Register{Address: addrDeviceTemp, Length: 1}, []uint16{0x231E}, "Ctrl 35 °C · Batt 30 °C"},
		{"charge stage mppt", &Register{Address: addrChargeLoadStatus, Length: 1}, []uint16{0x8002}, "MPPT · load on"},
		{"charge stage float", &Register{Address: addrChargeLoadStatus, Length: 1}, []uint16{0x0005}, "Float · load off"},
		{"controller faults ok", &Register{Address: addrControllerFaults, Length: 2}, []uint16{0, 0}, "OK"},
		{"current fault codes ok", &Register{Address: addrCurrentFaultCodes, Length: 4}, []uint16{0, 0, 0, 0}, "OK"},
		{"current fault codes", &Register{Address: addrCurrentFaultCodes, Length: 4}, []uint16{1, 0x14, 0, 0}, "Battery under-voltage; Inverter overload"},
		{"cutoff soc", &Register{Address: addrCutoffSOC, Length: 1}, []uint16{0x5A14}, "Charge cutoff 90% · discharge cutoff 20%"},
		{"current time", &Register{Address: addrCurrentTime, Length: 3}, EncodeTime(26, 9, 13, 14, 5, 9), "2026-09-13 14:05:09"},
		{"mfg date", &Register{Address: addrMfgDate, Length: 2}, []uint16{0x1905, 0x060E}, "2025-05-06 (built 14:00)"},
		{"protocol version", &Register{Address: addrProtocolVersion, Length: 2}, []uint16{100, 0}, "V1.00"},
		{"hex fallback multi-word", &Register{Address: 0x0018, Length: 2}, []uint16{0x1234, 0x5678}, "0x1234 0x5678"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecodeValue(tc.reg, tc.raw); got != tc.want {
				t.Errorf("DecodeValue() = %q, want %q", got, tc.want)
			}
		})
	}
}

func encodeLowByteString(s string) []uint16 {
	out := make([]uint16, 16)
	for i := 0; i < len(s) && i < len(out); i++ {
		out[i] = uint16(s[i])
	}
	return out
}

func TestDecodeControllerFaults(t *testing.T) {
	// B16 battery over-discharge + B21 controller temp too high → bits 0 and 5 of word1
	raw := []uint16{0x0000, 0x0000}
	raw[1] = 1 | (1 << 5)
	faults := DecodeControllerFaults(raw)
	if len(faults) != 2 {
		t.Fatalf("expected 2 faults, got %v", faults)
	}
	if faults[0] != "Battery over-discharge" || faults[1] != "Controller temperature too high" {
		t.Errorf("unexpected fault names: %v", faults)
	}
}

func TestDecodeFaultRecord(t *testing.T) {
	rec := DecodeFaultRecord(0, []uint16{1, 0x090D, 0x0E23, 0x2D00})
	if !rec.Valid || rec.Code != 1 || rec.CodeName != "Battery under-voltage" {
		t.Fatalf("unexpected record: %+v", rec)
	}
	if rec.Time != "09-13 14:35:45" {
		t.Errorf("Time = %q, want %q", rec.Time, "09-13 14:35:45")
	}
	invalid := DecodeFaultRecord(1, make([]uint16, 16))
	if invalid.Valid {
		t.Error("zero record should be invalid")
	}
	garbage := DecodeFaultRecord(2, []uint16{5, 0xFFFF, 0xFFFF, 0xFFFF, 1, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	if !garbage.Valid || garbage.Time != "" || len(garbage.Data) != 12 {
		t.Errorf("garbage time should be blank, got %+v", garbage)
	}
}

func TestParseValue(t *testing.T) {
	cases := []struct {
		name    string
		s       string
		reg     *Register
		want    uint16
		wantErr bool
	}{
		{"plain", "80", &Register{}, 80, false},
		{"hex", "0xAA", &Register{}, 0xAA, false},
		{"scaled", "14.4", &Register{Scale: 0.1}, 144, false},
		{"scaled round", "14.36", &Register{Scale: 0.1}, 144, false},
		{"negative scaled", "-30.0", &Register{Scale: 1, Signed: true}, 0xFFE2, false}, // -30
		{"negative plain", "-5", &Register{Signed: true}, 0xFFFB, false},
		{"overflow", "70000", &Register{}, 0, true},
		{"underflow", "-40000", &Register{Signed: true}, 0, true},
		{"garbage", "abc", &Register{}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseValue(tc.s, tc.reg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.s)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ParseValue(%q) = 0x%04X, want 0x%04X", tc.s, got, tc.want)
			}
		})
	}
}

func TestValidateValueSigned(t *testing.T) {
	reg := &Register{Scale: 1, Signed: true, Min: -40, Max: 100}
	raw, err := ParseValue("-30", reg)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateValue(reg, raw); err != nil {
		t.Errorf("ValidateValue(-30) failed: %v", err)
	}
	if err := ValidateValue(reg, 200); err == nil {
		t.Error("expected range error for 200")
	}
}

func TestPlanRegisterBlocks(t *testing.T) {
	regs := []*Register{}
	for _, addr := range []uint16{0x0100, 0x0101, 0x0102, 0x0104, 0x0105, 0x0107, 0x0108, 0x0109, 0x010B, 0x010C} {
		r := FindRegisterByAddr(addr)
		if r != nil {
			regs = append(regs, r)
		}
	}
	inv := []*Register{FindRegisterByAddr(0x0210), FindRegisterByAddr(0x0213), FindRegisterByAddr(0x0216)}
	regs = append(regs, inv...)

	blocks := PlanRegisterBlocks(regs, MaxBlockCount, MaxBlockGap)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks (controller + inverter), got %d: %+v", len(blocks), blocks)
	}
	if blocks[0].Start != 0x0100 || blocks[0].Count != 14 {
		t.Errorf("block0 = 0x%04X x%d, want 0x0100 x14", blocks[0].Start, blocks[0].Count)
	}
	if blocks[1].Start != 0x0210 || blocks[1].Count != 7 {
		t.Errorf("block1 = 0x%04X x%d, want 0x0210 x7", blocks[1].Start, blocks[1].Count)
	}

	// All settings registers must land in few blocks and cover every register.
	settings := []*Register{}
	for i := range AllRegisters {
		if AllRegisters[i].Category == "Battery Params" || AllRegisters[i].Category == "Inverter Settings" {
			settings = append(settings, &AllRegisters[i])
		}
	}
	blocks = PlanRegisterBlocks(settings, MaxBlockCount, MaxBlockGap)
	if len(blocks) > 3 {
		t.Errorf("settings should plan into ≤3 blocks, got %d", len(blocks))
	}
	total := 0
	for _, b := range blocks {
		if b.Count > MaxBlockCount {
			t.Errorf("block 0x%04X exceeds max count: %d", b.Start, b.Count)
		}
		total += len(b.Regs)
	}
	if total != len(settings) {
		t.Errorf("blocks cover %d of %d registers", total, len(settings))
	}
}

func TestFindRegister(t *testing.T) {
	if r := FindRegister("0xE001"); r == nil || r.Name != "PV Charge Current Limit" {
		t.Errorf("FindRegister by address failed: %+v", r)
	}
	if r := FindRegister("battery soc"); r == nil || r.Address != 0x0100 {
		t.Errorf("FindRegister by name failed: %+v", r)
	}
	if FindRegister("no-such-register") != nil {
		t.Error("expected nil for unknown register")
	}
}

func TestDecodeLowByteStringTrimsPadding(t *testing.T) {
	raw := []uint16{'S', 'R', 'N', 'E', 0x00, 0x00, 0x00, 0x00}
	if got := decodeLowByteString(raw); got != "SRNE" {
		t.Errorf("got %q", got)
	}
	if got := decodeLowByteString([]uint16{0xFF, 0xFF}); got != "---" {
		t.Errorf("erased string should be ---, got %q", got)
	}
	if !strings.Contains(decodeLowByteString([]uint16{0x01}), "?") {
		t.Error("control chars should be replaced")
	}
}
