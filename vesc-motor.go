package vesccan

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/go-daq/canbus"

	"go.viam.com/rdk/components/motor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
)

var (
	VescCanMotor = resource.NewModel("erh", "vesc-can", "vesc-can-motor")
)

func init() {
	resource.RegisterComponent(motor.API, VescCanMotor,
		resource.Registration[motor.Motor, *Config]{
			Constructor: newVescCanVescCanMotor,
		},
	)
}

const (
	CAN_PACKET_SET_DUTY                     = 0
	CAN_PACKET_SET_CURRENT                  = 1
	CAN_PACKET_SET_CURRENT_BRAKE            = 2
	CAN_PACKET_SET_RPM                      = 3
	CAN_PACKET_SET_POS                      = 4
	CAN_PACKET_FILL_RX_BUFFER               = 5
	CAN_PACKET_FILL_RX_BUFFER_LONG          = 6
	CAN_PACKET_PROCESS_RX_BUFFER            = 7
	CAN_PACKET_PROCESS_SHORT_BUFFER         = 8
	CAN_PACKET_STATUS                       = 9
	CAN_PACKET_SET_CURRENT_REL              = 10
	CAN_PACKET_SET_CURRENT_BRAKE_REL        = 11
	CAN_PACKET_SET_CURRENT_HANDBRAKE        = 12
	CAN_PACKET_SET_CURRENT_HANDBRAKE_REL    = 13
	CAN_PACKET_STATUS_2                     = 14
	CAN_PACKET_STATUS_3                     = 15
	CAN_PACKET_STATUS_4                     = 16
	CAN_PACKET_PING                         = 17
	CAN_PACKET_PONG                         = 18
	CAN_PACKET_DETECT_APPLY_ALL_FOC         = 19
	CAN_PACKET_DETECT_APPLY_ALL_FOC_RES     = 20
	CAN_PACKET_CONF_CURRENT_LIMITS          = 21
	CAN_PACKET_CONF_STORE_CURRENT_LIMITS    = 22
	CAN_PACKET_CONF_CURRENT_LIMITS_IN       = 23
	CAN_PACKET_CONF_STORE_CURRENT_LIMITS_IN = 24
	CAN_PACKET_CONF_FOC_ERPMS               = 25
	CAN_PACKET_CONF_STORE_FOC_ERPMS         = 26
	CAN_PACKET_STATUS_5                     = 27
)

type Config struct {
	Interface        string
	Id               int
	TicksPerRotation float64 `json:"ticks_per_rotation"`
}

func (cfg *Config) Validate(path string) ([]string, []string, error) {
	if cfg.Interface == "" {
		return nil, nil, fmt.Errorf("need an interface")
	}

	return nil, nil, nil
}

type vescCanVescCanMotor struct {
	resource.AlwaysRebuild

	name   resource.Name
	logger logging.Logger
	cfg    *Config

	cancelFunc func()

	socket *canbus.Socket

	statusMu sync.Mutex
	status   VESCStatus
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

	err = socket.Bind(conf.Interface)
	if err != nil {
		socket.Close()
		return nil, fmt.Errorf("failed to bind to CAN interface %s: %w", conf.Interface, err)
	}

	m := &vescCanVescCanMotor{
		name:       name,
		logger:     logger,
		cfg:        conf,
		cancelFunc: cancelFunc,
		socket:     socket,
	}

	go m.listenForMessages(cancelCtx)

	return m, nil
}

func (m *vescCanVescCanMotor) Name() resource.Name {
	return m.name
}

func (m *vescCanVescCanMotor) SetPower(ctx context.Context, duty float64, extra map[string]interface{}) error {
	if duty < -1.0 || duty > 1.0 {
		return fmt.Errorf("duty cycle must be between -1.0 and 1.0, got %f", duty)
	}

	scaledValue := int32(duty * 100000)
	return m.sendCommand(CAN_PACKET_SET_DUTY, scaledValue)
}

func (m *vescCanVescCanMotor) GoFor(ctx context.Context, rpm float64, revolutions float64, extra map[string]interface{}) error {
	pos, err := m.Position(ctx, extra)
	if err != nil {
		return err
	}
	pos += revolutions
	return m.GoTo(ctx, rpm, pos, extra)
}

func (m *vescCanVescCanMotor) GoTo(ctx context.Context, rpm float64, positionRevolutions float64, extra map[string]interface{}) error {
	if m.cfg.TicksPerRotation == 0 {
		return fmt.Errorf("need ticks_per_rotation")
	}
	pos := positionRevolutions / m.cfg.TicksPerRotation
	return m.SetPosition(int32(pos))
}

func (m *vescCanVescCanMotor) SetRPM(ctx context.Context, rpm float64, extra map[string]interface{}) error {
	return m.sendCommand(CAN_PACKET_SET_RPM, int32(rpm))
}

func (m *vescCanVescCanMotor) ResetZeroPosition(ctx context.Context, offset float64, extra map[string]interface{}) error {
	panic("not implemented")
}

func (m *vescCanVescCanMotor) Position(ctx context.Context, extra map[string]interface{}) (float64, error) {
	if m.cfg.TicksPerRotation == 0 {
		return 0, fmt.Errorf("need ticks_per_rotation")
	}

	m.statusMu.Lock()
	defer m.statusMu.Unlock()

	return float64(m.status.PIDPos) / m.cfg.TicksPerRotation, nil
}

func (m *vescCanVescCanMotor) Properties(ctx context.Context, extra map[string]interface{}) (motor.Properties, error) {
	return motor.Properties{
		PositionReporting: true,
	}, nil
}

func (m *vescCanVescCanMotor) Stop(ctx context.Context, extra map[string]interface{}) error {
	err := m.SetRPM(ctx, 0, extra)
	if err != nil {
		return err
	}
	return m.SetCurrentBrake(1)
}

func (m *vescCanVescCanMotor) IsPowered(ctx context.Context, extra map[string]interface{}) (bool, float64, error) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()

	return m.status.DutyCycle != 0, float64(m.status.DutyCycle), nil
}

func (m *vescCanVescCanMotor) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	mm := map[string]interface{}{}

	m.statusMu.Lock()
	defer m.statusMu.Unlock()

	mm["rpm"] = m.status.RPM
	mm["current"] = m.status.Current
	mm["duty_cycle"] = m.status.DutyCycle
	mm["amp_hours"] = m.status.AmpHours
	mm["amp_hours_charged"] = m.status.AmpHoursCharged
	mm["watt_hours"] = m.status.WattHours
	mm["watt_hours_chagred"] = m.status.WattHoursCharged
	mm["fet_temp"] = m.status.FETTemp
	mm["motor_temp"] = m.status.MotorTemp
	mm["current"] = m.status.CurrentIn
	mm["pid_pos"] = m.status.PIDPos
	mm["tachomoter"] = m.status.Tachometer
	mm["input_voltated"] = m.status.InputVoltage

	return mm, nil
}

func (m *vescCanVescCanMotor) IsMoving(ctx context.Context) (bool, error) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()

	return m.status.DutyCycle != 0, nil
}

func (m *vescCanVescCanMotor) buildExtendedID(command uint8) uint32 {
	// 29-bit Extended ID format:
	// Bits 28-26: 000 (frame control)
	// Bits 25-18: Command ID (8 bits)
	// Bits 17-16: 00 (spare)
	// Bits 15-8:  00000000 (reserved)
	// Bits 7-0:   VESC ID (8 bits)
	return uint32(command)<<8 | uint32(m.cfg.Id)
}

func (m *vescCanVescCanMotor) sendCommand(command uint8, value int32) error {
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, uint32(value))

	frame := canbus.Frame{
		ID:   m.buildExtendedID(command),
		Data: data,
		Kind: canbus.EFF, // Extended frame format
	}

	_, err := m.socket.Send(frame)
	return err
}

func (m *vescCanVescCanMotor) sendCommand8Byte(command uint8, data []byte) error {
	if len(data) != 8 {
		return fmt.Errorf("data must be exactly 8 bytes, got %d", len(data))
	}

	frame := canbus.Frame{
		ID:   m.buildExtendedID(command),
		Data: data,
		Kind: canbus.EFF, // Extended frame format
	}

	_, err := m.socket.Send(frame)
	return err
}

// SetCurrent sets the motor current in amperes
func (m *vescCanVescCanMotor) SetCurrent(current float32) error {
	scaledValue := int32(current * 1000) // Convert to milliamps
	return m.sendCommand(CAN_PACKET_SET_CURRENT, scaledValue)
}

// SetCurrentBrake sets the brake current in amperes
func (m *vescCanVescCanMotor) SetCurrentBrake(current float32) error {
	scaledValue := int32(current * 1000) // Convert to milliamps
	return m.sendCommand(CAN_PACKET_SET_CURRENT_BRAKE, scaledValue)
}

// SetPosition sets the target position (encoder steps)
func (m *vescCanVescCanMotor) SetPosition(position int32) error {
	return m.sendCommand(CAN_PACKET_SET_POS, position)
}

// SetCurrentRelative sets relative current (-1.0 to 1.0)
func (m *vescCanVescCanMotor) SetCurrentRelative(ratio float32) error {
	if ratio < -1.0 || ratio > 1.0 {
		return fmt.Errorf("current ratio must be between -1.0 and 1.0, got %f", ratio)
	}

	scaledValue := int32(ratio * 100000)
	return m.sendCommand(CAN_PACKET_SET_CURRENT_REL, scaledValue)
}

// SetCurrentBrakeRelative sets relative brake current (-1.0 to 1.0)
func (m *vescCanVescCanMotor) SetCurrentBrakeRelative(ratio float32) error {
	if ratio < -1.0 || ratio > 1.0 {
		return fmt.Errorf("brake current ratio must be between -1.0 and 1.0, got %f", ratio)
	}

	scaledValue := int32(ratio * 100000)
	return m.sendCommand(CAN_PACKET_SET_CURRENT_BRAKE_REL, scaledValue)
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

	return m.sendCommand8Byte(uint8(command), data)
}

func (m *vescCanVescCanMotor) Ping() error {
	return m.sendCommand(CAN_PACKET_PING, 0)
}

// GetStatus returns the latest status information (thread-safe)
func (m *vescCanVescCanMotor) GetStatus() VESCStatus {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	return m.status
}

func (m *vescCanVescCanMotor) IsAlive() bool {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	return time.Since(m.status.LastUpdate) < (time.Millisecond * 500)
}

// listenForMessages listens for incoming CAN messages from VESC
func (m *vescCanVescCanMotor) listenForMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			frame, err := m.socket.Recv()
			if err != nil {
				if ctx.Err() != nil {
					return // Context cancelled
				}
				m.logger.Warnf("Error receiving CAN frame: %v", err)
				continue
			}

			if frame.Kind == canbus.EFF {
				m.handleStatusMessage(frame)
			}
		}
	}
}

func (m *vescCanVescCanMotor) handleStatusMessage(frame canbus.Frame) {
	// Extract command and VESC ID from extended ID
	command := uint8((frame.ID >> 8) & 0xFF)
	senderID := uint8(frame.ID & 0xFF)

	if int(senderID) != m.cfg.Id {
		return
	}

	data := frame.Data

	if len(data) < 8 {
		m.logger.Warnf("not enough data command: %d", command)
		return
	}

	m.statusMu.Lock()
	defer m.statusMu.Unlock()

	switch command {
	case CAN_PACKET_STATUS:
		if len(data) >= 8 {
			m.status.RPM = int32(binary.BigEndian.Uint32(data[0:4]))
			m.status.Current = float32(int16(binary.BigEndian.Uint16(data[4:6]))) / 10.0
			m.status.DutyCycle = float32(int16(binary.BigEndian.Uint16(data[6:8]))) / 1000.0
		}

	case CAN_PACKET_STATUS_2:
		if len(data) >= 8 {
			m.status.AmpHours = float32(int32(binary.BigEndian.Uint32(data[0:4]))) / 10000.0
			m.status.AmpHoursCharged = float32(int32(binary.BigEndian.Uint32(data[4:8]))) / 10000.0
		}

	case CAN_PACKET_STATUS_3:
		if len(data) >= 8 {
			m.status.WattHours = float32(int32(binary.BigEndian.Uint32(data[0:4]))) / 10000.0
			m.status.WattHoursCharged = float32(int32(binary.BigEndian.Uint32(data[4:8]))) / 10000.0
		}

	case CAN_PACKET_STATUS_4:
		if len(data) >= 8 {
			m.status.FETTemp = float32(int16(binary.BigEndian.Uint16(data[0:2]))) / 10.0
			m.status.MotorTemp = float32(int16(binary.BigEndian.Uint16(data[2:4]))) / 10.0
			m.status.CurrentIn = float32(int16(binary.BigEndian.Uint16(data[4:6]))) / 10.0
			m.status.PIDPos = float32(int16(binary.BigEndian.Uint16(data[6:8]))) / 50.0
		}

	case CAN_PACKET_STATUS_5:
		if len(data) >= 8 {
			m.status.Tachometer = int32(binary.BigEndian.Uint32(data[0:4]))
			m.status.InputVoltage = float32(int16(binary.BigEndian.Uint16(data[4:6]))) / 10.0
		}

	case CAN_PACKET_PONG:
		m.logger.Infof("Received PONG from VESC ID %d", senderID)
		return
	}

	m.status.LastUpdate = time.Now()
}

func (m *vescCanVescCanMotor) Close(context.Context) error {
	m.cancelFunc()
	return m.socket.Close()
}
