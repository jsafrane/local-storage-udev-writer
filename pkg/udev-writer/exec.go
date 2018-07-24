package udevwriter

import (
	"fmt"
	osexec "os/exec"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
)

const (
	mountNsPath = "1/ns/mnt"
)

// ExecInterface is an wrapper around exec().
type ExecInterface interface {
	Exec(cmd []string) ([]byte, error)
}

// NewExec returns ExecInterface that runs exec().
func NewExec() ExecInterface {
	return &exec{}
}

type exec struct{}

var _ ExecInterface = &exec{}

func (*exec) Exec(cmd []string) ([]byte, error) {
	c := osexec.Command(cmd[0], cmd[1:]...)
	out, err := c.CombinedOutput()
	if err != nil {
		glog.V(4).Infof("executed %s: %s (error: %v)", strings.Join(cmd, " "), string(out), err)
	} else {
		glog.V(4).Infof("executed %s: %s (success)", strings.Join(cmd, " "), string(out))
	}
	return out, err
}

// NewNSEnterExec returns ExecInterface that exectes commands in the host mount
// namespace via nsenter.
func NewNSEnterExec(hostProcDir string) ExecInterface {
	return &nsenterExec{
		hostProcDir: hostProcDir,
	}
}

type nsenterExec struct {
	hostProcDir string
}

var _ ExecInterface = &nsenterExec{}

func (ne *nsenterExec) Exec(cmd []string) ([]byte, error) {
	hostProcMountNsPath := filepath.Join(ne.hostProcDir, mountNsPath)
	nsenterCmd := []string{"nsenter", fmt.Sprintf("--mount=%s", hostProcMountNsPath), "--"}
	nsenterCmd = append(nsenterCmd, cmd...)
	c := osexec.Command(nsenterCmd[0], nsenterCmd[1:]...)
	out, err := c.CombinedOutput()
	if err != nil {
		glog.V(4).Infof("executed %s: %s (error: %v)", strings.Join(nsenterCmd, " "), string(out), err)
	} else {
		glog.V(4).Infof("executed %s: %s (success)", strings.Join(nsenterCmd, " "), string(out))
	}
	return out, err
}
