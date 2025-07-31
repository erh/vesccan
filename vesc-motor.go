package vesccan

import (
	"context"
	"errors"

	"github.com/go-daq/canbus"

	"go.viam.com/rdk/components/motor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/utils/rpc"
)

var (
	VescCanMotor     = resource.NewModel("erh", "vesc-can", "vesc-can-motor")
	errUnimplemented = errors.New("unimplemented")
)

func init() {
	resource.RegisterComponent(motor.API, VescCanMotor,
		resource.Registration[motor.Motor, *Config]{
			Constructor: newVescCanVescCanMotor,
		},
	)
}

type Config struct {
	Interface string
	Id        int
}

func (cfg *Config) Validate(path string) ([]string, []string, error) {
	if cfg.Interface == "" {
		return nil, nil, fmt.Errorf("need an interface")
	}

	return nil, nil, nil
}

type vescCanVescCanMotor struct {
	resource.AlwaysRebuild

	name resource.Name
	logger logging.Logger
	cfg    *Config

	cancelFunc func()

	socket   *canbus.Socket
	vescID   uint8
	status   VESCStatus
	statusMu sync.RWMutex
}

func newVescCanVescCanMotor(ctx context.Context, deps resource.Dependencies, rawConf resource.Config, logger logging.Logger) (motor.Motor, error) {
	conf, err := resource.NativeConfig[*Config](rawConf)
	if err != nil {
		return nil, err
	}

	return NewVescCanMotor(ctx, deps, rawConf.ResourceName(), conf, logger)

}

func NewVescCanMotor(ctx context.Context, deps resource.Dependencies, name resource.Name, conf *Config, logger logging.Logger) (motor.Motor, error) {

	cancelCtx, cancelFunc := context.WithCancel(context.Background())

	socket, err := canbus.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create CAN socket: %w", err)
	}

	err := socket.Bind(conf.Interface)
	if err != nil {
		socket.Close()
		return nil, fmt.Errorf("failed to bind to CAN interface %s: %w", canInterface, err)
	}

	
	m := &vescCanVescCanMotor{
		name:       name,
		logger:     logger,
		cfg:        conf,
		cancelFunc: cancelFunc,
		socket:  socket,
		vescID:  vescID,

	}

	go m.listenForMessages(cancelContex)

	return m, nil
}

func (m *vescCanVescCanMotor) Name() resource.Name {
	return s.name
}

func (m *vescCanVescCanMotor) SetPower(ctx context.Context, duty float64, extra map[string]interface{}) error {
	if duty < -1.0 || duty > 1.0 {
		return fmt.Errorf("duty cycle must be between -1.0 and 1.0, got %f", duty)
	}
	
	scaledValue := int32(duty * 100000)
	return m.sendCommand(CAN_PACKET_SET_DUTY, scaledValue)
}

func (m *vescCanVescCanMotor) GoFor(ctx context.Context, rpm float64, revolutions float64, extra map[string]interface{}) error {
	panic("not implemented")
}

func (m *vescCanVescCanMotor) GoTo(ctx context.Context, rpm float64, positionRevolutions float64, extra map[string]interface{}) error {
	panic("not implemented")
}

func (m *vescCanVescCanMotor) SetRPM(ctx context.Context, rpm float64, extra map[string]interface{}) error {
	return v.sendCommand(CAN_PACKET_SET_RPM, rpm)
}

func (m *vescCanVescCanMotor) ResetZeroPosition(ctx context.Context, offset float64, extra map[string]interface{}) error {
	panic("not implemented")
}

func (m *vescCanVescCanMotor) Position(ctx context.Context, extra map[string]interface{}) (float64, error) {
	panic("not implemented")
}

func (m *vescCanVescCanMotor) Properties(ctx context.Context, extra map[string]interface{}) (motor.Properties, error) {
	panic("not implemented")
}

func (m *vescCanVescCanMotor) Stop(ctx context.Context, extra map[string]interface{}) error {
	panic("not implemented")
}

func (m *vescCanVescCanMotor) IsPowered(ctx context.Context, extra map[string]interface{}) (bool, float64, error) {
	panic("not implemented")
}

func (m *vescCanVescCanMotor) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	panic("not implemented")
}

func (m *vescCanVescCanMotor) IsMoving(ctx context.Context) (bool, error) {
	panic("not implemented")
}

func (m *vescCanVescCanMotor) buildExtendedID(command uint8) uint32 {
	// 29-bit Extended ID format:
	// Bits 28-26: 000 (frame control)
	// Bits 25-18: Command ID (8 bits)
	// Bits 17-16: 00 (spare)
	// Bits 15-8:  00000000 (reserved)
	// Bits 7-0:   VESC ID (8 bits)
	return uint32(command)<<8 | uint32(v.vescID)
}

func (m *vescCanVescCanMotor) sendCommand(command uint8, value int32) error {
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, uint32(value))

	frame := canbus.Frame{
		ID:   v.buildExtendedID(command),
		Data: data,
		Kind: canbus.EFF, // Extended frame format
	}

	_, err := v.socket.Send(frame)
	return err
}

func (m *vescCanVescCanMotor) sendCommand8Byte(command uint8, data []byte) error {
	if len(data) != 8 {
		return fmt.Errorf("data must be exactly 8 bytes, got %d", len(data))
	}

	frame := canbus.Frame{
		ID:   v.buildExtendedID(command),
		Data: data,
		Kind: canbus.EFF, // Extended frame format
	}

	_, err := v.socket.Send(frame)
	return err
}

// SetCurrent sets the motor current in amperes
func (m *vescCanVescCanMotor) SetCurrent(current float32) error {
	scaledValue := int32(current * 1000) // Convert to milliamps
	return v.sendCommand(CAN_PACKET_SET_CURRENT, scaledValue)
}

// SetCurrentBrake sets the brake current in amperes
func (m *vescCanVescCanMotor) SetCurrentBrake(current float32) error {
	scaledValue := int32(current * 1000) // Convert to milliamps
	return v.sendCommand(CAN_PACKET_SET_CURRENT_BRAKE, scaledValue)
}


// SetPosition sets the target position (encoder steps)
func (m *vescCanVescCanMotor) SetPosition(position int32) error {
	return v.sendCommand(CAN_PACKET_SET_POS, position)
}

// SetCurrentRelative sets relative current (-1.0 to 1.0)
func (m *vescCanVescCanMotor) SetCurrentRelative(ratio float32) error {
	if ratio < -1.0 || ratio > 1.0 {
		return fmt.Errorf("current ratio must be between -1.0 and 1.0, got %f", ratio)
	}
	
	scaledValue := int32(ratio * 100000)
	return v.sendCommand(CAN_PACKET_SET_CURRENT_REL, scaledValue)
}

// SetCurrentBrakeRelative sets relative brake current (-1.0 to 1.0)
func (m *vescCanVescCanMotor) SetCurrentBrakeRelative(ratio float32) error {
	if ratio < -1.0 || ratio > 1.0 {
		return fmt.Errorf("brake current ratio must be between -1.0 and 1.0, got %f", ratio)
	}
	
	scaledValue := int32(ratio * 100000)
	return v.sendCommand(CAN_PACKET_SET_CURRENT_BRAKE_REL, scaledValue)
}

// SetCurrentLimits sets the current limits (min and max in amperes)
func (m *vescCanVescCanMotor) SetCurrentLimits(minCurrent, maxCurrent float32, store bool) error {
	data := make([]byte, 8)
	
	// Convert to milliamps and store as big-endian 32-bit integers
	binary.BigEndian.PutUint32(data[0:4], uint32(minCurrent*1000))
	binary.BigEndian.PutUint32(data[4:8], uint32(maxCurrent*1000))
	
	command := CAN_PACKET_CONF_CURRENT_LIMITS
	if store {
		command = CAN_PACKET_CONF_STORE_CURRENT_LIMITS
	}
	
	return v.sendCommand8Byte(uint8(command), data)
}

func (m *vescCanVescCanMotor) Ping() error {
	return v.sendCommand(CAN_PACKET_PING, 0)
}

// GetStatus returns the latest status information (thread-safe)
func (m *vescCanVescCanMotor) GetStatus() VESCStatus {
	v.statusMu.RLock()
	defer v.statusMu.RUnlock()
	return v.status
}

func (m *vescCanVescCanMotor) IsAlive() bool {
	v.statusMu.RLock()
	defer v.statusMu.RUnlock()
	return time.Since(v.status.LastUpdate) < (time.Millisecond * 500)
}

// updateStatus updates the status in a thread-safe manner
func (m *vescCanVescCanMotor) updateStatus(updateFunc func(*VESCStatus)) {
	v.statusMu.Lock()
	defer v.statusMu.Unlock()
	updateFunc(&v.status)
	v.status.LastUpdate = time.Now()
}

// listenForMessages listens for incoming CAN messages from VESC
func (m *vescCanVescCanMotor) listenForMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			frame, err := v.socket.Recv()
			if err != nil {
				if ctx.Err() != nil {
					return // Context cancelled
				}
				log.Printf("Error receiving CAN frame: %v", err)
				continue
			}

			if frame.Kind == canbus.EFF {
				v.handleStatusMessage(frame)
			}
		}
	}
}

func (m *vescCanVescCanMotor) handleStatusMessage(frame canbus.Frame) {
	// Extract command and VESC ID from extended ID
	command := uint8((frame.ID >> 8) & 0xFF)
	senderID := uint8(frame.ID & 0xFF)

	if senderID != v.vescID {
		return
	}

	data := frame.Data
	
	switch command {
	case CAN_PACKET_STATUS:
		if len(data) >= 8 {
			v.updateStatus(func(s *VESCStatus) {
				s.RPM = int32(binary.BigEndian.Uint32(data[0:4]))
				s.Current = float32(int16(binary.BigEndian.Uint16(data[4:6]))) / 10.0
				s.DutyCycle = float32(int16(binary.BigEndian.Uint16(data[6:8]))) / 1000.0
			})
		}

	case CAN_PACKET_STATUS_2:
		if len(data) >= 8 {
			v.updateStatus(func(s *VESCStatus) {
				s.AmpHours = float32(int32(binary.BigEndian.Uint32(data[0:4]))) / 10000.0
				s.AmpHoursCharged = float32(int32(binary.BigEndian.Uint32(data[4:8]))) / 10000.0
			})
		}

	case CAN_PACKET_STATUS_3:
		if len(data) >= 8 {
			v.updateStatus(func(s *VESCStatus) {
				s.WattHours = float32(int32(binary.BigEndian.Uint32(data[0:4]))) / 10000.0
				s.WattHoursCharged = float32(int32(binary.BigEndian.Uint32(data[4:8]))) / 10000.0
			})
		}

	case CAN_PACKET_STATUS_4:
		if len(data) >= 8 {
			v.updateStatus(func(s *VESCStatus) {
				s.FETTemp = float32(int16(binary.BigEndian.Uint16(data[0:2]))) / 10.0
				s.MotorTemp = float32(int16(binary.BigEndian.Uint16(data[2:4]))) / 10.0
				s.CurrentIn = float32(int16(binary.BigEndian.Uint16(data[4:6]))) / 10.0
				s.PIDPos = float32(int16(binary.BigEndian.Uint16(data[6:8]))) / 50.0
			})
		}

	case CAN_PACKET_STATUS_5:
		if len(data) >= 8 {
			v.updateStatus(func(s *VESCStatus) {
				s.Tachometer = int32(binary.BigEndian.Uint32(data[0:4]))
				s.InputVoltage = float32(int16(binary.BigEndian.Uint16(data[4:6]))) / 10.0
			})
		}

	case CAN_PACKET_PONG:
		log.Printf("Received PONG from VESC ID %d", senderID)
	}
}

func (m *vescCanVescCanMotor) Close(context.Context) error {
	m.cancelFunc()
	return m.socket.Close()
}
