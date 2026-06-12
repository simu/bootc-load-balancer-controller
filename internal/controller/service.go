package controller

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type NodeService (string)

const (
	HAProxy        = NodeService("haproxy")
	Keepalived     = NodeService("keepalived")
	Conntrackd     = NodeService("conntrackd")
	NetworkManager = NodeService("networkmanager")
	Sysctl         = NodeService("sysctl")
	Firewalld      = NodeService("firewalld")
)

func (s NodeService) ReloadCommand() (string, error) {
	commandString := ""
	switch s {
	case HAProxy:
		commandString = "systemctl reload haproxy"
	case Keepalived:
		commandString = "systemctl reload keepalived"
	case Conntrackd:
		commandString = "systemctl restart conntrackd"
	case NetworkManager:
		commandString = "nmcli connection reload"
	case Sysctl:
		commandString = "sysctl --system"
	case Firewalld:
		commandString = "firewall-cmd --reload"
	default:
		return "", fmt.Errorf("Missing reload/restart command for %s", s)
	}
	return commandString, nil
}

func (s NodeService) Reload(ctx context.Context) error {
	l := logf.FromContext(ctx)

	commandString, err := s.ReloadCommand()
	if err != nil {
		return err
	}

	command := strings.Split(commandString, " ")
	cmd := exec.Command(command[0], command[1:]...)

	var cmdout strings.Builder
	var cmderr strings.Builder
	cmd.Stdout = &cmdout
	cmd.Stderr = &cmderr

	err = cmd.Run()
	l.Info("command output", "out", cmdout, "err", cmderr)
	if err != nil {
		return fmt.Errorf("restart/reload of %s failed: %w", s, err)
	}
	return nil
}
