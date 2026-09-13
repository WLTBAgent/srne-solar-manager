package controller

import "strings"

// AllRegisters — complete register map extracted from the Modbus V1.3 PDF.
// Read-only (status) registers are included for monitoring;
// RW and W registers can be modified via the CLI.
var AllRegisters = []Register{
	// ─── P00: Product Info (0x000A–0x0049) ───
	{Address: 0x000A, Length: 1, Name: "Max Voltage & Rated Charge Current", Access: "R", Category: "Product Info",
		Description: "Packed: system voltage (high byte) + rated charge current (low byte)"},
	{Address: 0x000B, Length: 1, Name: "Product Type", Access: "R", Category: "Product Info",
		Enum: map[uint16]string{0: "Controller (Home)", 1: "Controller (Street Light)", 3: "Inverter", 4: "Integrated Inverter Controller", 5: "Mains Freq Off-Grid"}},
	{Address: 0x000C, Length: 8, Name: "Product Model", Access: "R", Category: "Product Info"},
	{Address: 0x0014, Length: 2, Name: "Software Version", Access: "R", Category: "Product Info"},
	{Address: 0x0016, Length: 2, Name: "Hardware Version", Access: "R", Category: "Product Info"},
	{Address: 0x0018, Length: 2, Name: "Product SN", Access: "R", Category: "Product Info"},
	{Address: 0x001A, Length: 1, Name: "Controller Device Address", Access: "R", Category: "Product Info"},
	{Address: 0x001B, Length: 1, Name: "Model Code", Access: "R", Category: "Product Info"},
	{Address: 0x001C, Length: 2, Name: "RS485 Protocol Version", Access: "R", Category: "Product Info"},
	{Address: 0x001E, Length: 2, Name: "Date of Manufacture", Access: "R", Category: "Product Info"},
	{Address: 0x0020, Length: 1, Name: "Production Site Code", Access: "R", Category: "Product Info",
		Enum: map[uint16]string{0: "Shenzhen", 1: "Dongguan"}},
	{Address: 0x0021, Length: 20, Name: "Software Compilation Time", Access: "R", Category: "Product Info"},
	{Address: 0x0035, Length: 20, Name: "Product SN String", Access: "R", Category: "Product Info"},

	// ─── P01: Controller Data (0x0100–0x0121) ───
	{Address: 0x0100, Length: 1, Name: "Battery SOC", Access: "R", Unit: "%", Scale: 1, Category: "Controller Data"},
	{Address: 0x0101, Length: 1, Name: "Battery Voltage", Access: "R", Unit: "V", Scale: 0.1, Category: "Controller Data"},
	{Address: 0x0102, Length: 1, Name: "Charge Current", Access: "R", Unit: "A", Scale: 0.1, Signed: true, Invert: true, Category: "Controller Data",
		Description: "Battery current; positive = charging. Device reports discharge-positive (two's complement), inverted here for display."},
	{Address: 0x0103, Length: 1, Name: "Controller/Battery Temperature", Access: "R", Unit: "°C", Category: "Controller Data",
		Description: "High 8 bits: controller temp; Low 8 bits: battery temp"},
	{Address: 0x0104, Length: 1, Name: "Load DC Voltage", Access: "R", Unit: "V", Scale: 0.1, Category: "Controller Data"},
	{Address: 0x0105, Length: 1, Name: "Load DC Current", Access: "R", Unit: "A", Scale: 0.01, Category: "Controller Data"},
	{Address: 0x0106, Length: 1, Name: "Load DC Power", Access: "R", Unit: "W", Category: "Controller Data"},
	{Address: 0x0107, Length: 1, Name: "PV Panel Voltage", Access: "R", Unit: "V", Scale: 0.1, Category: "Controller Data"},
	{Address: 0x0108, Length: 1, Name: "PV Panel Current", Access: "R", Unit: "A", Scale: 0.1, Category: "Controller Data"},
	{Address: 0x0109, Length: 1, Name: "PV Charge Power", Access: "R", Unit: "W", Category: "Controller Data"},
	{Address: 0x010A, Length: 1, Name: "DC Load On/Off", Access: "W", Category: "Controller Data",
		Enum:        map[uint16]string{0: "Off", 1: "On"},
		Description: "Write 1=on, 0=off"},
	{Address: 0x010B, Length: 1, Name: "Load & Charge Status", Access: "R", Category: "Controller Data",
		Description: "Low 8 bits charge status: 0=off,1=start,2=MPPT,3=equalize,4=boost,5=float,6=current limit. High 8 bits: b7=load status, b0-b6=brightness"},
	{Address: 0x010C, Length: 2, Name: "Controller Fault/Alarm", Access: "R", Category: "Controller Data",
		Description: "Bit flags: B16=battery over-discharge … B31=reserved"},

	// ─── P02: Inverter Data (0x0200–0x023B) ───
	{Address: 0x0200, Length: 4, Name: "Current Fault Bits", Access: "R", Category: "Inverter Data"},
	{Address: 0x0204, Length: 4, Name: "Current Fault Code", Access: "R", Category: "Inverter Data"},
	{Address: 0x020C, Length: 3, Name: "Current Time", Access: "RW", Category: "Inverter Data",
		Description: "0x020C: year/month, 0x020D: day/hour, 0x020E: min/sec"},
	{Address: 0x0210, Length: 1, Name: "Machine State", Access: "R", Category: "Inverter Data",
		Enum: map[uint16]string{0: "Power-up delay", 1: "Waiting", 2: "Init", 3: "Soft start", 4: "Mains operation", 5: "Inverter operation", 6: "Inverter→Mains", 7: "Mains→Inverter", 10: "Shutdown", 11: "Fault"}},
	{Address: 0x0211, Length: 1, Name: "Password Protection Status", Access: "R", Category: "Inverter Data",
		Enum: map[uint16]string{0: "No password entered", 1: "User password entered", 4: "Manufacturer password entered"}},
	{Address: 0x0212, Length: 1, Name: "Bus Voltage", Access: "R", Unit: "V", Scale: 0.1, Category: "Inverter Data"},
	{Address: 0x0213, Length: 1, Name: "Grid Voltage", Access: "R", Unit: "V", Scale: 0.1, Category: "Inverter Data"},
	{Address: 0x0214, Length: 1, Name: "Grid Current", Access: "R", Unit: "A", Scale: 0.1, Category: "Inverter Data"},
	{Address: 0x0215, Length: 1, Name: "Grid Frequency", Access: "R", Unit: "Hz", Scale: 0.01, Category: "Inverter Data"},
	{Address: 0x0216, Length: 1, Name: "Inverter Voltage", Access: "R", Unit: "V", Scale: 0.1, Category: "Inverter Data"},
	{Address: 0x0217, Length: 1, Name: "Inverter Current", Access: "R", Unit: "A", Scale: 0.1, Category: "Inverter Data"},
	{Address: 0x0218, Length: 1, Name: "Inverter Frequency", Access: "R", Unit: "Hz", Scale: 0.01, Category: "Inverter Data"},
	{Address: 0x0219, Length: 1, Name: "Load Current", Access: "R", Unit: "A", Scale: 0.1, Category: "Inverter Data"},
	{Address: 0x021A, Length: 1, Name: "Load PF", Access: "R", Scale: 0.01, Signed: true, Category: "Inverter Data"},
	{Address: 0x021B, Length: 1, Name: "Load Active Power", Access: "R", Unit: "W", Signed: true, Category: "Inverter Data"},
	{Address: 0x021C, Length: 1, Name: "Load Apparent Power", Access: "R", Unit: "VA", Category: "Inverter Data"},
	{Address: 0x021D, Length: 1, Name: "Inverter DC Component", Access: "R", Unit: "mV", Signed: true, Category: "Inverter Data"},
	{Address: 0x021E, Length: 1, Name: "Mains Charge Current", Access: "R", Unit: "A", Scale: 0.1, Category: "Inverter Data"},
	{Address: 0x021F, Length: 1, Name: "Load Ratio", Access: "R", Unit: "%", Category: "Inverter Data"},
	{Address: 0x0220, Length: 1, Name: "Heat Sink A Temp", Access: "R", Unit: "°C", Scale: 0.1, Signed: true, Category: "Inverter Data"},
	{Address: 0x0221, Length: 1, Name: "Heat Sink B Temp", Access: "R", Unit: "°C", Scale: 0.1, Signed: true, Category: "Inverter Data"},
	{Address: 0x0222, Length: 1, Name: "Heat Sink C Temp", Access: "R", Unit: "°C", Scale: 0.1, Signed: true, Category: "Inverter Data"},
	{Address: 0x0223, Length: 1, Name: "Ambient Temperature", Access: "R", Unit: "°C", Scale: 0.1, Signed: true, Category: "Inverter Data"},
	{Address: 0x0224, Length: 1, Name: "PV Buck Current 1", Access: "R", Unit: "A", Scale: 0.1, Category: "Inverter Data"},
	{Address: 0x0225, Length: 1, Name: "Buck Current 2", Access: "R", Unit: "A", Scale: 0.1, Category: "Inverter Data"},

	// ─── P03: Device Control (0xDF00–0xDF1F) — Write Only ───
	{Address: 0xDF00, Length: 1, Name: "Power ON/OFF", Access: "W", Category: "Device Control",
		Enum: map[uint16]string{0: "Power Off", 1: "Power On"}},
	{Address: 0xDF01, Length: 1, Name: "Reset", Access: "W", Category: "Device Control",
		Enum: map[uint16]string{1: "Reset"}},
	{Address: 0xDF02, Length: 1, Name: "Restore Defaults", Access: "W", Category: "Device Control",
		Enum:        map[uint16]string{0xAA: "Restore"},
		Description: "Clears accumulated info, restores defaults. Restart to take effect."},
	{Address: 0xDF03, Length: 1, Name: "Clear Alarm", Access: "W", Category: "Device Control",
		Enum: map[uint16]string{1: "Clear"}},
	{Address: 0xDF04, Length: 1, Name: "Clear Statistics", Access: "W", Category: "Device Control",
		Enum: map[uint16]string{1: "Clear"}},
	{Address: 0xDF05, Length: 1, Name: "Clear History", Access: "W", Category: "Device Control",
		Enum: map[uint16]string{1: "Clear"}},
	{Address: 0xDF06, Length: 2, Name: "Firmware Upgrade", Access: "W", Category: "Device Control"},
	{Address: 0xDF08, Length: 1, Name: "Sleep/Wake", Access: "W", Category: "Device Control",
		Enum: map[uint16]string{0x5A5A: "Sleep", 0xA5A5: "Wake"}},
	{Address: 0xDF09, Length: 3, Name: "Manual Light-Up", Access: "W", Category: "Device Control",
		Description: "Sub-addr 0: switch 1=on/0=off, Sub-addr 1: power 0–100%, Sub-addr 2: time 0–54000s"},
	{Address: 0xDF0C, Length: 1, Name: "Generator Switch", Access: "W", Category: "Device Control",
		Enum: map[uint16]string{0: "No action", 1: "Switch to generator"}},
	{Address: 0xDF0D, Length: 1, Name: "Immediate Equalizing Charge", Access: "W", Category: "Device Control",
		Enum: map[uint16]string{0: "Disable", 1: "Enable"}},

	// ─── P05: Battery Parameters (0xE000–0xE025) — Read/Write ───
	{Address: 0xE000, Length: 1, Name: "Reserved (E000)", Access: "RW", Category: "Battery Params"},
	{Address: 0xE001, Length: 1, Name: "PV Charge Current Limit", Access: "RW", Unit: "A", Min: 0, Max: 100, Default: 80, Category: "Battery Params",
		Description: "1st gen max 50A, 2nd gen max 70A"},
	{Address: 0xE002, Length: 1, Name: "Nominal Battery Capacity", Access: "RW", Unit: "AH", Min: 0, Max: 400, Default: 100, Category: "Battery Params"},
	{Address: 0xE003, Length: 1, Name: "System Voltage Setup", Access: "RW", Unit: "V", Min: 12, Max: 255, Default: 48, Category: "Battery Params",
		Enum: map[uint16]string{12: "12V", 24: "24V", 36: "36V", 48: "48V", 255: "Auto"}},
	{Address: 0xE004, Length: 1, Name: "Battery Type", Access: "RW", Min: 0, Max: 10, Default: 3, Category: "Battery Params",
		Enum: map[uint16]string{0: "Lead-Acid", 1: "Lithium", 2: "Custom", 3: "Default"}},
	{Address: 0xE005, Length: 1, Name: "Over Voltage", Access: "RW", Unit: "V", Scale: 0.1, Min: 9.0, Max: 15.5, Default: 15.5, Category: "Battery Params",
		Description: "Per cell — battery overcharge protection, fast protection"},
	{Address: 0xE006, Length: 1, Name: "Limited Charge Voltage", Access: "RW", Unit: "V", Scale: 0.1, Min: 9.0, Max: 15.5, Default: 14.4, Category: "Battery Params"},
	{Address: 0xE007, Length: 1, Name: "Equalizing Charge Voltage", Access: "RW", Unit: "V", Scale: 0.1, Min: 9.0, Max: 15.5, Default: 14.4, Category: "Battery Params"},
	{Address: 0xE008, Length: 1, Name: "Boost Charge Voltage", Access: "RW", Unit: "V", Scale: 0.1, Min: 9.0, Max: 15.5, Default: 14.4, Category: "Battery Params",
		Description: "Overcharge voltage for lithium battery"},
	{Address: 0xE009, Length: 1, Name: "Floating Charge Voltage", Access: "RW", Unit: "V", Scale: 0.1, Min: 9.0, Max: 15.5, Default: 14.0, Category: "Battery Params",
		Description: "Overcharge return voltage for lithium battery"},
	{Address: 0xE00A, Length: 1, Name: "Boost Charge Return Voltage", Access: "RW", Unit: "V", Scale: 0.1, Min: 9.0, Max: 15.5, Default: 13.2, Category: "Battery Params"},
	{Address: 0xE00B, Length: 1, Name: "Over Discharge Return Voltage", Access: "RW", Unit: "V", Scale: 0.1, Min: 9.0, Max: 15.5, Default: 12.6, Category: "Battery Params"},
	{Address: 0xE00C, Length: 1, Name: "Under-Voltage Warning", Access: "RW", Unit: "V", Scale: 0.1, Min: 9.0, Max: 15.5, Default: 11.0, Category: "Battery Params",
		Description: "Low battery voltage alarm, load not cut off"},
	{Address: 0xE00D, Length: 1, Name: "Over Discharge Voltage", Access: "RW", Unit: "V", Scale: 0.1, Min: 9.0, Max: 15.5, Default: 12.2, Category: "Battery Params",
		Description: "Low battery voltage alarm, load cut off"},
	{Address: 0xE00E, Length: 1, Name: "Limited Discharge Voltage", Access: "RW", Unit: "V", Scale: 0.1, Min: 9.0, Max: 15.5, Default: 11.2, Category: "Battery Params"},
	{Address: 0xE00F, Length: 1, Name: "Charge/Discharge Cutoff SOC", Access: "RW", Unit: "%", Min: 0, Max: 100, Category: "Battery Params",
		Description: "High 8 bits: charge cutoff SOC; Low 8 bits: discharge cutoff SOC"},
	{Address: 0xE010, Length: 1, Name: "Over Discharge Delay", Access: "RW", Unit: "s", Min: 0, Max: 120, Default: 60, Category: "Battery Params"},
	{Address: 0xE011, Length: 1, Name: "Equalizing Charge Time", Access: "RW", Unit: "min", Min: 0, Max: 600, Default: 120, Category: "Battery Params",
		Description: "Step +10"},
	{Address: 0xE012, Length: 1, Name: "Boost Charge Time", Access: "RW", Unit: "min", Min: 10, Max: 600, Default: 120, Category: "Battery Params",
		Description: "Step +10"},
	{Address: 0xE013, Length: 1, Name: "Equalizing Charge Interval", Access: "RW", Unit: "days", Min: 0, Max: 255, Default: 30, Category: "Battery Params"},
	{Address: 0xE014, Length: 1, Name: "Temperature Compensation Coeff", Access: "RW", Unit: "mV/°C/2V", Min: 0, Max: 10, Default: 5, Signed: true, Category: "Battery Params",
		Description: "Lead-acid only"},
	{Address: 0xE015, Length: 1, Name: "Charge Upper Limit Temp", Access: "RW", Unit: "°C", Min: -40, Max: 100, Default: 60, Signed: true, Category: "Battery Params"},
	{Address: 0xE016, Length: 1, Name: "Charge Lower Limit Temp", Access: "RW", Unit: "°C", Min: -40, Max: 100, Default: -30, Signed: true, Category: "Battery Params"},
	{Address: 0xE017, Length: 1, Name: "Discharge Upper Limit Temp", Access: "RW", Unit: "°C", Min: -40, Max: 100, Default: 60, Signed: true, Category: "Battery Params"},
	{Address: 0xE018, Length: 1, Name: "Discharge Lower Limit Temp", Access: "RW", Unit: "°C", Min: -40, Max: 100, Default: -30, Signed: true, Category: "Battery Params"},
	{Address: 0xE019, Length: 1, Name: "Heating Start Temperature", Access: "RW", Unit: "°C", Min: -40, Max: 100, Default: 0, Signed: true, Category: "Battery Params",
		Description: "Lead-acid only"},
	{Address: 0xE01A, Length: 1, Name: "Heating Stop Temperature", Access: "RW", Unit: "°C", Min: -40, Max: 100, Default: 5, Signed: true, Category: "Battery Params",
		Description: "Lead-acid only"},
	{Address: 0xE01B, Length: 1, Name: "Mains Switching Voltage", Access: "RW", Unit: "V", Scale: 0.1, Min: 9.0, Max: 15.5, Default: 11.5, Category: "Battery Params",
		Description: "Load switches to mains when battery below this point"},
	{Address: 0xE01C, Length: 1, Name: "Stop Charging Current", Access: "RW", Unit: "A", Scale: 0.1, Min: 0.0, Max: 40.0, Default: 0.0, Category: "Battery Params",
		Description: "Lithium only — stops charging when current drops below this in CV state"},
	{Address: 0xE01D, Length: 1, Name: "DC Load Working Mode", Access: "RW", Min: 0, Max: 17, Default: 0, Category: "Battery Params",
		Description: "0=light control only, 1-14=light on + N hour delay, 15=manual, 16=test, 17=steady on"},
	{Address: 0xE01E, Length: 1, Name: "Light Control Delay Time", Access: "RW", Unit: "min", Min: 0, Max: 60, Default: 0, Category: "Battery Params"},
	{Address: 0xE01F, Length: 1, Name: "Light Control Voltage", Access: "RW", Unit: "V", Min: 1, Max: 40, Default: 5, Category: "Battery Params"},
	{Address: 0xE020, Length: 1, Name: "Batteries in Series", Access: "RW", Min: 1, Max: 200, Default: 4, Category: "Battery Params",
		Description: "Number of lithium batteries in series"},
	{Address: 0xE021, Length: 1, Name: "Special Power Control", Access: "RW", Category: "Battery Params",
		Description: "b8=nightly load on, b3=heating, b2=no charge below 0°C, b0-b1=charge mode (00=direct, 01=PWM)"},
	{Address: 0xE022, Length: 1, Name: "Inverter Switching Voltage", Access: "RW", Unit: "V", Scale: 0.1, Min: 9.0, Max: 15.5, Default: 14.0, Category: "Battery Params",
		Description: "Switch back to inverter when battery rises above this"},
	{Address: 0xE023, Length: 1, Name: "Equalizing Charge Timeout", Access: "RW", Unit: "min", Min: 5, Max: 900, Default: 240, Category: "Battery Params",
		Description: "Step +5"},
	{Address: 0xE024, Length: 1, Name: "Lithium Battery Activation Current", Access: "RW", Unit: "A", Scale: 0.1, Min: 0, Max: 10.0, Default: 2.5, Category: "Battery Params"},
	{Address: 0xE025, Length: 1, Name: "Reserved (E025)", Access: "R", Category: "Battery Params"},

	// ─── P07: Inverter User Settings (0xE200–0xE215) — Read/Write ───
	{Address: 0xE200, Length: 1, Name: "Inverter RS485 Address", Access: "RW", Min: 1, Max: 254, Default: 1, Category: "Inverter Settings"},
	{Address: 0xE201, Length: 1, Name: "Baud Rate", Access: "RW", Min: 48, Max: 384, Default: 96, Category: "Inverter Settings",
		Description: "48=4800, 96=9600, 192=19200, 384=38400"},
	{Address: 0xE202, Length: 1, Name: "User Password Set", Access: "W", Min: 0, Max: 65535, Default: 0, Category: "Inverter Settings",
		Description: "4-digit decimal password. 0=no password"},
	{Address: 0xE203, Length: 1, Name: "Password Input", Access: "W", Min: 0, Max: 65535, Default: 0, Category: "Inverter Settings"},
	{Address: 0xE204, Length: 1, Name: "Output Priority", Access: "RW", Min: 0, Max: 2, Default: 1, Category: "Inverter Settings",
		Enum: map[uint16]string{0: "Solar", 1: "Line (mains first)", 2: "SBU (Solar-Battery-Utility)"}},
	{Address: 0xE205, Length: 1, Name: "Mains Charge Current Limit", Access: "RW", Unit: "A", Scale: 0.1, Min: 0, Max: 100, Default: 80, Category: "Inverter Settings"},
	{Address: 0xE206, Length: 1, Name: "Equalizing Charge Enable", Access: "RW", Min: 0, Max: 1, Default: 0, Category: "Inverter Settings",
		Enum: map[uint16]string{0: "Disable", 1: "Enable"}},
	{Address: 0xE207, Length: 1, Name: "Eco Threshold", Access: "RW", Unit: "W", Min: 0, Max: 1000, Default: 25, Category: "Inverter Settings"},
	{Address: 0xE208, Length: 1, Name: "Output Voltage", Access: "RW", Unit: "V", Scale: 0.1, Min: 100.0, Max: 264.0, Default: 230, Category: "Inverter Settings"},
	{Address: 0xE209, Length: 1, Name: "Output Frequency", Access: "RW", Unit: "Hz", Scale: 0.01, Min: 45.0, Max: 65.0, Default: 50.0, Category: "Inverter Settings"},
	{Address: 0xE20A, Length: 1, Name: "Maximum Charge Current", Access: "RW", Unit: "A", Scale: 0.1, Min: 0, Max: 150, Default: 100, Category: "Inverter Settings"},
	{Address: 0xE20B, Length: 1, Name: "AC Input Range", Access: "RW", Min: 0, Max: 1, Default: 1, Category: "Inverter Settings",
		Enum: map[uint16]string{0: "Wide Range", 1: "Narrow Range"}},
	{Address: 0xE20C, Length: 1, Name: "Eco Mode", Access: "RW", Min: 0, Max: 1, Default: 0, Category: "Inverter Settings",
		Enum: map[uint16]string{0: "Disable", 1: "Enable"}},
	{Address: 0xE20D, Length: 1, Name: "Overload Auto Restart", Access: "RW", Min: 0, Max: 1, Default: 1, Category: "Inverter Settings",
		Enum: map[uint16]string{0: "Disable", 1: "Enable"}},
	{Address: 0xE20E, Length: 1, Name: "Over-Temp Auto Restart", Access: "RW", Min: 0, Max: 1, Default: 1, Category: "Inverter Settings",
		Enum: map[uint16]string{0: "Disable", 1: "Enable"}},
	{Address: 0xE20F, Length: 1, Name: "Charge Priority", Access: "RW", Min: 0, Max: 3, Default: 2, Category: "Inverter Settings",
		Enum: map[uint16]string{0: "PV preferred (mains when PV absent)", 1: "Mains preferred (PV when mains absent)", 2: "Hybrid (PV preferred, simultaneous)", 3: "PV only (no mains charge)"}},
	{Address: 0xE210, Length: 1, Name: "Alarm Control", Access: "RW", Min: 0, Max: 1, Default: 1, Category: "Inverter Settings",
		Enum: map[uint16]string{0: "Disable", 1: "Enable"}},
	{Address: 0xE211, Length: 1, Name: "Alarm on Input Interrupt", Access: "RW", Min: 0, Max: 1, Default: 1, Category: "Inverter Settings",
		Enum: map[uint16]string{0: "Disable", 1: "Enable"}},
	{Address: 0xE212, Length: 1, Name: "Overload Bypass Enable", Access: "RW", Min: 0, Max: 1, Default: 1, Category: "Inverter Settings",
		Enum: map[uint16]string{0: "Disable", 1: "Enable"}},
	{Address: 0xE213, Length: 1, Name: "Record Fault Code", Access: "RW", Min: 0, Max: 1, Default: 1, Category: "Inverter Settings",
		Enum: map[uint16]string{0: "Disable", 1: "Enable"}},
	{Address: 0xE214, Length: 1, Name: "Split-Phase Transformer", Access: "RW", Min: 0, Max: 1, Default: 1, Category: "Inverter Settings",
		Enum: map[uint16]string{0: "Disable", 1: "Enable"}},
	{Address: 0xE215, Length: 1, Name: "Reserved (E215)", Access: "RW", Min: 0, Max: 1, Default: 1, Category: "Inverter Settings"},

	// ─── P08: Power Statistics / History (0xF000–0xF3FF) — Read Only ───
	{Address: 0xF000, Length: 7, Name: "PV Generation (7-day history)", Access: "R", Unit: "AH", Category: "Statistics"},
	{Address: 0xF007, Length: 7, Name: "Battery Charge (7-day history)", Access: "R", Unit: "AH", Category: "Statistics"},
	{Address: 0xF00E, Length: 7, Name: "Battery Discharge (7-day history)", Access: "R", Unit: "AH", Category: "Statistics"},
	{Address: 0xF015, Length: 7, Name: "Mains Charge (7-day history)", Access: "R", Unit: "AH", Category: "Statistics"},
	{Address: 0xF01C, Length: 7, Name: "Load Power Consumption (7-day history)", Access: "R", Unit: "kWh", Scale: 0.1, Category: "Statistics"},
	{Address: 0xF023, Length: 7, Name: "Load Power from Mains (7-day history)", Access: "R", Unit: "kWh", Scale: 0.1, Category: "Statistics"},
	{Address: 0xF02D, Length: 1, Name: "Battery Charge AH (today)", Access: "R", Unit: "AH", Category: "Statistics"},
	{Address: 0xF02E, Length: 1, Name: "Battery Discharge AH (today)", Access: "R", Unit: "AH", Category: "Statistics"},
	{Address: 0xF02F, Length: 1, Name: "PV Generation (today)", Access: "R", Unit: "kWh", Scale: 0.1, Category: "Statistics"},
	{Address: 0xF030, Length: 1, Name: "Load Power Consumption (today)", Access: "R", Unit: "kWh", Scale: 0.1, Category: "Statistics"},
	{Address: 0xF031, Length: 1, Name: "Total Running Days", Access: "R", Unit: "days", Category: "Statistics"},
	{Address: 0xF032, Length: 1, Name: "Total Battery Over-Discharge Count", Access: "R", Category: "Statistics"},
	{Address: 0xF033, Length: 1, Name: "Total Battery Full Charge Count", Access: "R", Category: "Statistics"},
	{Address: 0xF034, Length: 2, Name: "Accumulated Battery Charge AH", Access: "R", Unit: "AH", Category: "Statistics"},
	{Address: 0xF036, Length: 2, Name: "Accumulated Battery Discharge AH", Access: "R", Unit: "AH", Category: "Statistics"},
	{Address: 0xF038, Length: 2, Name: "Accumulated PV Generation", Access: "R", Unit: "kWh", Scale: 0.1, Category: "Statistics"},
	{Address: 0xF03A, Length: 2, Name: "Accumulated Load Power Consumption", Access: "R", Unit: "kWh", Scale: 0.1, Category: "Statistics"},
	{Address: 0xF03C, Length: 1, Name: "Mains Charge (today)", Access: "R", Unit: "AH", Category: "Statistics"},
	{Address: 0xF03D, Length: 1, Name: "Load Power from Mains (today)", Access: "R", Unit: "kWh", Scale: 0.1, Category: "Statistics"},
	{Address: 0xF03E, Length: 1, Name: "Inverter Working Hours (today)", Access: "R", Unit: "min", Category: "Statistics"},
	{Address: 0xF03F, Length: 1, Name: "Bypass Working Hours (today)", Access: "R", Unit: "min", Category: "Statistics"},
	{Address: 0xF040, Length: 3, Name: "Power-On Time", Access: "R", Category: "Statistics"},
	{Address: 0xF043, Length: 3, Name: "Last Equalizing Charge Completion Time", Access: "R", Category: "Statistics"},
	{Address: 0xF046, Length: 2, Name: "Accumulated Mains Charge", Access: "R", Unit: "kWh", Scale: 0.1, Category: "Statistics"},
	{Address: 0xF048, Length: 2, Name: "Accumulated Load Power from Battery", Access: "R", Unit: "kWh", Scale: 0.1, Category: "Statistics"},
	{Address: 0xF04A, Length: 1, Name: "Accumulated Inverter Working Hours", Access: "R", Unit: "h", Category: "Statistics"},
	{Address: 0xF04B, Length: 1, Name: "Accumulated Bypass Working Hours", Access: "R", Unit: "h", Category: "Statistics"},

	// ─── P09: Fault History (0xF800–0xF9FF) ───
	{Address: 0xF800, Length: 16, Name: "Fault Record 0", Access: "RW", Category: "Fault History"},
	{Address: 0xF810, Length: 16, Name: "Fault Record 1", Access: "RW", Category: "Fault History"},
	{Address: 0xF820, Length: 16, Name: "Fault Record 2", Access: "RW", Category: "Fault History"},
	{Address: 0xF830, Length: 16, Name: "Fault Record 3", Access: "RW", Category: "Fault History"},
	{Address: 0xF840, Length: 16, Name: "Fault Record 4", Access: "RW", Category: "Fault History"},
	{Address: 0xF850, Length: 16, Name: "Fault Record 5", Access: "RW", Category: "Fault History"},
	{Address: 0xF860, Length: 16, Name: "Fault Record 6", Access: "RW", Category: "Fault History"},
	{Address: 0xF870, Length: 16, Name: "Fault Record 7", Access: "RW", Category: "Fault History"},
	{Address: 0xF880, Length: 16, Name: "Fault Record 8", Access: "RW", Category: "Fault History"},
	{Address: 0xF890, Length: 16, Name: "Fault Record 9", Access: "RW", Category: "Fault History"},
	{Address: 0xF8A0, Length: 16, Name: "Fault Record 10", Access: "RW", Category: "Fault History"},
	{Address: 0xF8B0, Length: 16, Name: "Fault Record 11", Access: "RW", Category: "Fault History"},
	{Address: 0xF8C0, Length: 16, Name: "Fault Record 12", Access: "RW", Category: "Fault History"},
	{Address: 0xF8D0, Length: 16, Name: "Fault Record 13", Access: "RW", Category: "Fault History"},
	{Address: 0xF8E0, Length: 16, Name: "Fault Record 14", Access: "RW", Category: "Fault History"},
	{Address: 0xF8F0, Length: 16, Name: "Fault Record 15", Access: "RW", Category: "Fault History"},
}

// ConfigurableRegisters returns only registers that can be written (RW or W).
func ConfigurableRegisters() []Register {
	var out []Register
	for _, r := range AllRegisters {
		if r.Access == "RW" || r.Access == "W" {
			out = append(out, r)
		}
	}
	return out
}

// ReadableConfigurableRegisters returns registers that are both
// configurable and readable (RW). Write-only registers cannot be
// read back, so they are excluded from value polling.
func ReadableConfigurableRegisters() []Register {
	var out []Register
	for _, r := range AllRegisters {
		if r.Access == "RW" {
			out = append(out, r)
		}
	}
	return out
}

// RegistersByCategory groups all registers by category.
func RegistersByCategory() map[string][]Register {
	out := make(map[string][]Register)
	for _, r := range AllRegisters {
		out[r.Category] = append(out[r.Category], r)
	}
	return out
}

// FindRegister looks up a register by hex address string (e.g. "0xE001") or name.
func FindRegister(query string) *Register {
	// Try address match
	addr, err := ParseAddress(query)
	if err == nil {
		for i := range AllRegisters {
			if AllRegisters[i].Address == addr {
				return &AllRegisters[i]
			}
		}
	}
	// Try case-insensitive name match
	q := strings.ToLower(query)
	for i := range AllRegisters {
		if strings.Contains(strings.ToLower(AllRegisters[i].Name), q) {
			return &AllRegisters[i]
		}
	}
	return nil
}

// FindRegisterByAddr looks up a register by uint16 address.
func FindRegisterByAddr(addr uint16) *Register {
	for i := range AllRegisters {
		if AllRegisters[i].Address == addr {
			return &AllRegisters[i]
		}
	}
	return nil
}

// EnumSortedKeys returns sorted enum keys.
func EnumSortedKeys(m map[uint16]string) []uint16 {
	keys := make([]uint16, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

// StatusAddresses are the key status register addresses for live monitoring.
var StatusAddresses = []uint16{
	0x0100, 0x0101, 0x0102, 0x0103,
	0x0104, 0x0105, 0x0106,
	0x0107, 0x0108, 0x0109,
	0x010B,
	0x0210, 0x0212, 0x0213, 0x0215, 0x0216, 0x0219, 0x021B, 0x021F,
	0x0220, 0x0223,
}

// ControlAction defines a write-only device command.
type ControlAction struct {
	Addr uint16
	Val  uint16
	Desc string
}

// ControlActions maps action names to their register writes.
var ControlActions = map[string]ControlAction{
	"power-on":         {0xDF00, 1, "Power ON"},
	"power-off":        {0xDF00, 0, "Power OFF"},
	"reset":            {0xDF01, 1, "Reset"},
	"restore-defaults": {0xDF02, 0xAA, "Restore factory defaults"},
	"clear-alarm":      {0xDF03, 1, "Clear current alarm"},
	"clear-stats":      {0xDF04, 1, "Clear statistics"},
	"clear-history":    {0xDF05, 1, "Clear history"},
	"sleep":            {0xDF08, 0x5A5A, "Sleep mode"},
	"wake":             {0xDF08, 0xA5A5, "Wake up"},
	"equalize-on":      {0xDF0D, 1, "Immediate equalizing charge: ON"},
	"equalize-off":     {0xDF0D, 0, "Immediate equalizing charge: OFF"},
	"dc-load-on":       {0x010A, 1, "DC load ON"},
	"dc-load-off":      {0x010A, 0, "DC load OFF"},
}
