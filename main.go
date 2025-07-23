package main

import (
	"os"

	"github.com/coldzerofear/vgpu-manager-scheduler-plugin/pkg/plugin"
	"k8s.io/component-base/cli"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"
)

func main() {
	command := app.NewSchedulerCommand(
		app.WithPlugin(plugin.Name, plugin.New),
	)
	code := cli.Run(command)
	os.Exit(code)
}
