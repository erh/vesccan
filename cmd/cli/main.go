package main

import (
	"context"
	"flag"

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

	thing, err := vesccan.NewVescCanMotor(ctx, deps, motor.Named("foo"), &cfg, logger)
	if err != nil {
		return err
	}
	defer thing.Close(ctx)

	return nil
}
