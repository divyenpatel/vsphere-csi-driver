package windows

import (
	"os"
	"os/exec"
)

func RunPowershellCmd(command string, envs ...string) ([]byte, error) {
	cmd := exec.Command("powershell", "-Mta", "-NoProfile", "-Command", command)
	cmd.Env = append(os.Environ(), envs...)
	out, err := cmd.CombinedOutput()
	return out, err
}
