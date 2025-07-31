package vesccan

// VESCStatus represents telemetry data from VESC
type VESCStatus struct {
	// Status 1
	RPM         int32
	Current     float32
	DutyCycle   float32
	
	// Status 2
	AmpHours         float32
	AmpHoursCharged  float32
	
	// Status 3
	WattHours        float32
	WattHoursCharged float32
	
	// Status 4
	FETTemp     float32
	MotorTemp   float32
	CurrentIn   float32
	PIDPos      float32
	
	// Status 5
	Tachometer   int32
	InputVoltage float32
	
	LastUpdate time.Time
}
