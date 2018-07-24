package udevwriter

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"text/template"
	"time"

	"github.com/golang/glog"
)

const (
	udevRulesTemplate = "99-kubernetes-%s.rules"
	udevFileTemplate  = `
# Generated file, do not modify!

# Skip non-block devices.
SUBSYSTEM!="block", GOTO="out"

{{.Rules}}

# Check if the device was matched by any of the rules above.
ENV{KUBERNETES_STORAGE_CLASS}=="", GOTO="out"

# The device matched, create a symlink with stable device name if possible.
ENV{ID_SERIAL}!="", SYMLINK+="disk/kubernetes/{{.Name}}/$env{KUBERNETES_STORAGE_CLASS}/$env{ID_BUS}-$env{ID_SERIAL}"

# Fall back to kernel name if stable device name does not exist.
ENV{ID_SERIAL}=="", SYMLINK+="disk/kubernetes/{{.Name}}/$env{KUBERNETES_STORAGE_CLASS}/$kernel"

LABEL="out"
`
)

type UdevSync struct {
	// Unique name of node group, reused as rules filename.
	name string

	// Path to ConfigMap file with udev rules from the operator.
	configFile string

	// Last known content of config file.
	oldConfigContent []byte

	// Path to output udev rules file.
	rulesFile string

	// Template of udev rules file.
	udevTemplate *template.Template

	// Exec interface to use
	exec ExecInterface
}

func NewUdevSync(configFile, rulesPath, name string, exec ExecInterface) *UdevSync {
	rulesFile := path.Join(rulesPath, fmt.Sprintf(udevRulesTemplate, name))
	glog.V(4).Infof("using rules file %s", rulesFile)
	return &UdevSync{
		name:         name,
		configFile:   configFile,
		rulesFile:    rulesFile,
		udevTemplate: template.Must(template.New("rules").Parse(udevFileTemplate)),
		exec:         exec,
	}
}

func (u *UdevSync) Run(stopCh <-chan struct{}) {
	// TODO: use inotify
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	err := u.applyConfig()
	if err != nil {
		glog.Infof("failed to apply config: %s", err)
	}

	for {
		select {
		case <-ticker.C:
			err := u.applyConfig()
			if err != nil {
				glog.Infof("failed to apply config: %s", err)
			}

		case <-stopCh:
			glog.Infof("stopping")
			u.removeUdevFile()
			return
		}
	}
}

func (u *UdevSync) applyConfig() error {
	config, err := ioutil.ReadFile(u.configFile)
	if err != nil {
		return err
	}

	if !u.needApplyConfig(config) {
		glog.V(4).Info("no change detected, skipping config update")
		return nil
	}

	if err := u.writeRulesFile(config); err != nil {
		return err
	}

	if err := u.reloadUdev(); err != nil {
		return fmt.Errorf("failed to reload udev rules: %s", err)
	}

	u.oldConfigContent = config
	glog.V(2).Info("configuration applied")
	return nil
}

func (u *UdevSync) writeRulesFile(config []byte) error {
	out, err := os.OpenFile(u.rulesFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer out.Close()

	params := struct {
		Name  string
		Rules string
	}{
		u.name,
		string(config),
	}

	if err := u.udevTemplate.Execute(out, params); err != nil {
		return fmt.Errorf("failed to execute template: %s", err)
	}
	glog.V(2).Infof("configuration written to %s", u.rulesFile)
	return nil
}

func (u *UdevSync) removeUdevFile() {
	if err := os.Remove(u.rulesFile); err != nil {
		glog.Infof("error removing udev file %s: %s", u.rulesFile, err)
		return
	} else {
	}
	glog.V(2).Infof("udev file %s removed", u.rulesFile)

	if err := u.reloadUdev(); err != nil {
		glog.Infof("error reloading udev: %s", err)
	} else {
		glog.V(2).Infof("udev reloaded")
	}
}

func (u *UdevSync) reloadUdev() error {
	commands := [][]string{
		// Reload all rules files.
		{"udevadm", "control", "--reload"},
		// Pass all block devices through newly loaded rules
		{"udevadm", "trigger", "--subsystem-match=block"},
		// Block until all devices are processed
		{"udevadm", "settle", "--timeout=300"},
	}

	for _, cmd := range commands {
		out, err := u.exec.Exec(cmd)
		if err != nil {
			return fmt.Errorf("error executing %s: %s (%s))",
				strings.Join(cmd, " "),
				string(out),
				err)
		}
	}
	return nil
}

func (u *UdevSync) needApplyConfig(config []byte) bool {
	if len(config) != len(u.oldConfigContent) {
		return true
	}
	for i := range config {
		if config[i] != u.oldConfigContent[i] {
			return true
		}
	}
	return false
}
