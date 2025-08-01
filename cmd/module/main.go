package main

import (
	"go.viam.com/rdk/components/motor"
	"go.viam.com/rdk/module"
	"go.viam.com/rdk/resource"
	"vesccan"
)

func main() {
	// ModularMain can take multiple APIModel arguments, if your module implements multiple models.
	module.ModularMain(resource.APIModel{motor.API, vesccan.VescCanMotor})
}
