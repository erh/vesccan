package main

import (
	"context"
	"flag"
	"time"

	"go.viam.com/rdk/components/motor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"vesccan"
)

func main() {
	err := realMain()
	if err != nil {
		panic(err)
	}
}

func realMain() error {
	canInterface := flag.String("interface", "", "can interface")
	id := flag.Int("id", 0, "motor id")

	flag.Parse()

	ctx := context.Background()
	logger := logging.NewLogger("cli")

	cfg := vesccan.Config{
		Interface: *canInterface,
		Id:        *id,
	}
	_, _, err := cfg.Validate("")
	if err != nil {
		return err
	}

	deps := resource.Dependencies{}

	m, err := vesccan.NewVescCanMotor(ctx, deps, motor.Named("foo"), &cfg, logger)
	if err != nil {
		return err
	}
	defer m.Close(ctx)

	logger.Infof("Setting power")
	err = m.SetPower(ctx, .25, nil)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 2)

	logger.Infof("Setting RPM")
	err = m.SetRPM(ctx, 60, nil)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 2)

	return nil
}
