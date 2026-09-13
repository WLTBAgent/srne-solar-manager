package controller

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Register addresses with special (non-generic) value encodings, taken
// from the "Modbus Register Address of integrated inverter controller
// V1.3" specification.
const (
	addrRatedSys          = 0x000A // high byte: system voltage, low byte: rated charge current
	addrProductModel      = 0x000C // ASCII, low byte per register
	addrSoftwareVersion   = 0x0014 // word0: CPU1 version, word1: CPU2 (100 → V1.00)
	addrHardwareVersion   = 0x0016 // word0: control board, word1: power board
	addrMfgDate           = 0x001E // word0: year/month, word1: day/hour
	addrProtocolVersion   = 0x001C // word0: protocol version (100 → V1.00)
	addrCompileTime       = 0x0021 // ASCII, low byte per register
	addrProductSN         = 0x0035 // ASCII, low byte per register
	addrDeviceTemp        = 0x0103 // high byte: controller temp, low byte: battery temp
	addrChargeLoadStatus  = 0x010B // low byte: charge stage, high bit7: load on
	addrControllerFaults  = 0x010C // 2 words, bits 16..31 defined
	addrCurrentFaultBits  = 0x0200 // 4 words of internal debug fault bits
	addrCurrentFaultCodes = 0x0204 // 4 words, each one fault code (0 = none)
	addrCurrentTime       = 0x020C // year/month, day/hour, min/sec
	addrCutoffSOC         = 0xE00F // high byte: charge cutoff SOC, low byte: discharge
	addrPowerOnTime       = 0xF040 // date/time, 3 words
	addrLastEqualize      = 0xF043 // date/time, 3 words
)

// chargeStageNames decodes the low byte of 0x010B.
var chargeStageNames = map[byte]string{
	0: "Not charging",
	1: "Start",
	2: "MPPT",
	3: "Equalize",
	4: "Boost",
	5: "Float",
	6: "Current limit",
}

// controllerFaultBits decodes bits 16..31 of 0x010C (word1<<16 | word0).
// Definitions per the V1.3 spec, page P01.
var controllerFaultBits = []struct {
	bit  uint
	name string
}{
	{16, "Battery over-discharge"},
	{17, "Battery over-voltage"},
	{18, "Under-voltage warning"},
	{19, "Load short circuit"},
	{20, "Load over-power / over-current"},
	{21, "Controller temperature too high"},
	{22, "Ambient temperature too high"},
	{23, "PV input power too high"},
	{24, "PV input short circuit"},
	{25, "PV input over-voltage"},
	{26, "PV panel counter-current"},
	{27, "PV operating point over-voltage"},
	{28, "PV panel reversed polarity"},
	{29, "Anti-reverse MOS short circuit"},
	{30, "Charge MOS short circuit"},
}

// inverterFaultCodeNames holds the few codes the Modbus spec itself
// documents (in the 0x0204 example). The full table lives in the device
// instruction manual, so unknown codes are shown numerically.
var inverterFaultCodeNames = map[uint16]string{
	0x01: "Battery under-voltage",
	0x14: "Inverter overload",
}

// ─── Value decoding ───

// invalidRaw is the word SRNE devices report when a reading is not
// available (powered-down section, unreachable source).
const invalidRaw = 0xFFFF

// DecodeValue converts raw uint16(s) into a human-readable string.
// Registers with special encodings (packed bytes, ASCII strings,
// versions, timestamps, fault words) are decoded explicitly; everything
// else falls through to enum/scale/unit formatting.
func DecodeValue(reg *Register, raw []uint16) string {
	if len(raw) == 0 {
		return "---"
	}
	if !reg.Signed && allInvalid(raw) {
		return "n/a"
	}
	if s, ok := decodeSpecial(reg, raw); ok {
		return s
	}

	val := raw[0]

	if reg.Enum != nil {
		if label, ok := reg.Enum[val]; ok {
			return label
		}
	}

	// 2-word statistics registers are big-endian 32-bit accumulators.
	if reg.Category == "Statistics" && reg.Length == 2 && len(raw) == 2 {
		combined := uint64(raw[0])<<16 | uint64(raw[1])
		if isScaled(reg) {
			return fmt.Sprintf("%s%s", formatScaled(float64(combined)*reg.Scale, reg), unitSuffix(reg))
		}
		return fmt.Sprintf("%d%s", combined, unitSuffix(reg))
	}

	// Other multi-word registers without a special decoder render as
	// hex words (the spec's %x display format).
	if reg.Length > 1 {
		parts := make([]string, len(raw))
		for i, w := range raw {
			parts[i] = fmt.Sprintf("0x%04X", w)
		}
		return strings.Join(parts, " ")
	}

	if reg.Signed {
		sval := int16(val)
		if reg.Invert {
			sval = -sval
		}
		if isScaled(reg) {
			return fmt.Sprintf("%s%s", formatScaled(float64(sval)*reg.Scale, reg), unitSuffix(reg))
		}
		return fmt.Sprintf("%d%s", sval, unitSuffix(reg))
	}

	if isScaled(reg) {
		return fmt.Sprintf("%s%s", formatScaled(float64(val)*reg.Scale, reg), unitSuffix(reg))
	}

	return fmt.Sprintf("%d%s", val, unitSuffix(reg))
}

func isScaled(reg *Register) bool {
	return reg.Scale != 0 && reg.Scale != 1
}

// formatScaled renders a display-unit float with decimals matching the
// register magnification (0.1 → 1 decimal, 0.01 → 2 decimals).
func formatScaled(v float64, reg *Register) string {
	switch reg.Scale {
	case 0.1:
		return fmt.Sprintf("%.1f", v)
	case 0.01:
		return fmt.Sprintf("%.2f", v)
	default:
		return strconv.FormatFloat(v, 'g', -1, 64)
	}
}

func unitSuffix(reg *Register) string {
	if reg.Unit == "" {
		return ""
	}
	return " " + reg.Unit
}

// decodeSpecial returns the decoded representation of registers whose
// raw words do not follow the plain enum/scale format.
func decodeSpecial(reg *Register, raw []uint16) (string, bool) {
	switch reg.Address {
	case addrProductModel, addrCompileTime, addrProductSN:
		return decodeLowByteString(raw), true
	case addrSoftwareVersion:
		return fmt.Sprintf("CPU1 %s · CPU2 %s", formatVersion(raw[0]), formatVersionAt(raw, 1)), true
	case addrHardwareVersion:
		return fmt.Sprintf("Control %s · Power %s", formatVersion(raw[0]), formatVersionAt(raw, 1)), true
	case addrRatedSys:
		sys, amps := raw[0]>>8, raw[0]&0xFF
		if sys == 0xFF {
			return fmt.Sprintf("Auto system voltage · rated %d A", amps), true
		}
		return fmt.Sprintf("%d V system · rated %d A", sys, amps), true
	case addrMfgDate:
		return fmt.Sprintf("20%02d-%02d-%02d (built %02d:00)",
			raw[0]>>8, raw[0]&0xFF, raw[1]>>8, raw[1]&0xFF), true
	case addrProtocolVersion:
		return formatVersion(raw[0]), true
	case addrDeviceTemp:
		ctrl, batt := raw[0]>>8, raw[0]&0xFF
		return fmt.Sprintf("Ctrl %s · Batt %s", tempOrNA(byte(ctrl)), tempOrNA(byte(batt))), true
	case addrChargeLoadStatus:
		return decodeChargeLoadStatus(raw[0]), true
	case addrControllerFaults:
		faults := DecodeControllerFaults(raw)
		if len(faults) == 0 {
			return "OK", true
		}
		return strings.Join(faults, "; "), true
	case addrCurrentFaultCodes:
		var names []string
		for _, code := range raw {
			if code != 0 {
				names = append(names, DecodeFaultCode(code))
			}
		}
		if len(names) == 0 {
			return "OK", true
		}
		return strings.Join(names, "; "), true
	case addrCurrentTime, addrPowerOnTime, addrLastEqualize:
		if len(raw) < 3 {
			return "", false
		}
		y, mo, d, h, mi, s := DecodeTime(raw)
		return fmt.Sprintf("20%02d-%02d-%02d %02d:%02d:%02d", y, mo, d, h, mi, s), true
	case addrCutoffSOC:
		return fmt.Sprintf("Charge cutoff %d%% · discharge cutoff %d%%", raw[0]>>8, raw[0]&0xFF), true
	}
	return "", false
}

// decodeLowByteString decodes registers where only the low 8 bits of
// each word carry an ASCII character (Product Model, SN string, …).
func decodeLowByteString(raw []uint16) string {
	var b strings.Builder
	for _, w := range raw {
		c := byte(w & 0xFF)
		if c == 0x00 || c == 0xFF {
			break // NUL / erased padding terminates the string
		}
		if c < 0x20 || c > 0x7E {
			c = '?'
		}
		b.WriteByte(c)
	}
	s := strings.TrimSpace(b.String())
	if s == "" {
		return "---"
	}
	return s
}

// formatVersion renders an integer like 100 as "V1.00" per the spec.
func formatVersion(v uint16) string {
	return fmt.Sprintf("V%d.%02d", v/100, v%100)
}

func formatVersionAt(raw []uint16, i int) string {
	if i < len(raw) && raw[i] != 0 {
		return formatVersion(raw[i])
	}
	return "—"
}

// allInvalid reports whether every word is the device's "not
// available" marker (0xFFFF).
func allInvalid(raw []uint16) bool {
	for _, w := range raw {
		if w != invalidRaw {
			return false
		}
	}
	return true
}

// tempOrNA renders one packed temperature byte; 0xFF means no sensor.
func tempOrNA(c byte) string {
	if c == 0xFF {
		return "—"
	}
	return fmt.Sprintf("%d °C", c)
}

func decodeChargeLoadStatus(v uint16) string {
	stage, ok := chargeStageNames[byte(v&0xFF)]
	if !ok {
		stage = fmt.Sprintf("Stage %d", v&0xFF)
	}
	load := "load off"
	if v&0x8000 != 0 {
		load = "load on"
	}
	return fmt.Sprintf("%s · %s", stage, load)
}

// DecodeControllerFaults returns the human names of all controller fault
// bits set in the 2-word register 0x010C.
func DecodeControllerFaults(raw []uint16) []string {
	if len(raw) < 2 {
		return nil
	}
	bits := uint32(raw[1])<<16 | uint32(raw[0])
	var out []string
	for _, f := range controllerFaultBits {
		if bits&(1<<f.bit) != 0 {
			out = append(out, f.name)
		}
	}
	return out
}

// DecodeFaultCode names an inverter fault code; unknown codes are shown
// numerically rather than guessed.
func DecodeFaultCode(code uint16) string {
	if name, ok := inverterFaultCodeNames[code]; ok {
		return name
	}
	return fmt.Sprintf("Fault 0x%02X", code)
}

// FaultRecord is one decoded entry of the fault history area (0xF800+).
type FaultRecord struct {
	Index    int
	Valid    bool
	Code     uint16
	CodeName string
	Time     string // "MM-DD HH:MM:SS", empty when not reported
	Data     []uint16
}

// DecodeFaultRecord decodes one 16-word fault history record.
// Word 0 is the fault code (0 = invalid record), words 1–3 the
// occurrence time (month/day, hour/minute, second/-), words 4–15 the
// twelve data samples captured at the moment of the fault.
func DecodeFaultRecord(index int, raw []uint16) FaultRecord {
	rec := FaultRecord{Index: index}
	if len(raw) < 4 || raw[0] == 0 {
		return rec
	}
	rec.Valid = true
	rec.Code = raw[0]
	rec.CodeName = DecodeFaultCode(rec.Code)
	if t := decodeShortTime(raw[1], raw[2], raw[3]); t != "" {
		rec.Time = t
	}
	if len(raw) > 4 {
		rec.Data = append(rec.Data, raw[4:]...)
	}
	return rec
}

// decodeShortTime packs (month/day, hour/minute, second/…) into a
// string, validating ranges so garbage words render as hex instead.
func decodeShortTime(w1, w2, w3 uint16) string {
	mo, d := int(w1>>8), int(w1&0xFF)
	h, mi := int(w2>>8), int(w2&0xFF)
	s := int(w3 >> 8)
	if mo < 1 || mo > 12 || d < 1 || d > 31 || h > 23 || mi > 59 || s > 59 {
		return ""
	}
	return fmt.Sprintf("%02d-%02d %02d:%02d:%02d", mo, d, h, mi, s)
}

// ─── Value parsing ───

// ParseAddress parses hex ("0xE001") or decimal ("57345") address strings to uint16.
func ParseAddress(s string) (uint16, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, err := strconv.ParseUint(s[2:], 16, 16)
		if err != nil {
			return 0, err
		}
		return uint16(v), nil
	}
	v, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(v), nil
}

// ParseValue parses a user-supplied value string into a uint16 raw
// register value, applying the register scale and handling signed
// ranges via two's complement.
func ParseValue(s string, reg *Register) (uint16, error) {
	s = strings.TrimSpace(s)

	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, err := strconv.ParseUint(s[2:], 16, 16)
		if err != nil {
			return 0, fmt.Errorf("invalid hex value: %s", s)
		}
		return uint16(v), nil
	}

	scale := 1.0
	if isScaled(reg) {
		scale = reg.Scale
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid value: %s", s)
	}
	raw := math.Round(f / scale)
	if raw > 65535 || raw < -32768 {
		return 0, fmt.Errorf("value %s out of 16-bit range", s)
	}
	if raw < 0 {
		return uint16(int16(raw)), nil
	}
	return uint16(raw), nil
}

// RawToFloat converts raw uint16 to float64 applying scale and sign.
func RawToFloat(reg *Register, raw []uint16) float64 {
	if len(raw) == 0 {
		return 0
	}
	var v float64
	if reg.Signed {
		v = float64(int16(raw[0]))
	} else {
		v = float64(raw[0])
	}
	if isScaled(reg) {
		v *= reg.Scale
	}
	return v
}

// FloatToRaw converts a display float64 to raw uint16.
func FloatToRaw(reg *Register, f float64) uint16 {
	if isScaled(reg) {
		return uint16(math.Round(f / reg.Scale))
	}
	return uint16(math.Round(f))
}

// ValidateValue checks that a raw value is within the register's min/max range.
func ValidateValue(reg *Register, raw uint16) error {
	if reg.Min == 0 && reg.Max == 0 {
		return nil
	}
	displayVal := float64(raw)
	if reg.Signed {
		displayVal = float64(int16(raw))
	}
	if isScaled(reg) {
		displayVal *= reg.Scale
	}
	if displayVal < reg.Min {
		return fmt.Errorf("value %.1f is below minimum %.1f", displayVal, reg.Min)
	}
	if displayVal > reg.Max {
		return fmt.Errorf("value %.1f is above maximum %.1f", displayVal, reg.Max)
	}
	return nil
}

// EncodeTime packs year/month/day/hour/min/sec into the 3-register format for 0x020C.
func EncodeTime(year, month, day, hour, min, sec int) []uint16 {
	r1 := uint16((year << 8) | (month & 0xFF))
	r2 := uint16((day << 8) | (hour & 0xFF))
	r3 := uint16((min << 8) | (sec & 0xFF))
	return []uint16{r1, r2, r3}
}

// DecodeTime unpacks 3 raw uint16 values from 0x020C.
func DecodeTime(raw []uint16) (year, month, day, hour, min, sec int) {
	if len(raw) >= 1 {
		year = int(raw[0]>>8) & 0xFF
		month = int(raw[0]) & 0xFF
	}
	if len(raw) >= 2 {
		day = int(raw[1]>>8) & 0xFF
		hour = int(raw[1]) & 0xFF
	}
	if len(raw) >= 3 {
		min = int(raw[2]>>8) & 0xFF
		sec = int(raw[2]) & 0xFF
	}
	return
}

// BytesToUint16 converts big-endian byte pairs to uint16 slice.
func BytesToUint16(b []byte) []uint16 {
	if len(b)%2 != 0 {
		return nil
	}
	out := make([]uint16, len(b)/2)
	for i := range out {
		out[i] = binary.BigEndian.Uint16(b[i*2:])
	}
	return out
}
