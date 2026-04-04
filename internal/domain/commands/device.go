package commands

import (
	"fmt"
	"os"

	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// DeviceCommand manages device registration in the sync state.
type DeviceCommand struct {
	stateRepo repositories.StateRepository
}

// NewDeviceCommand creates a new DeviceCommand.
func NewDeviceCommand(stateRepo repositories.StateRepository) *DeviceCommand {
	return &DeviceCommand{stateRepo: stateRepo}
}

// List prints all registered devices.
func (c *DeviceCommand) List(repoPath string) error {
	state, err := c.stateRepo.Load(repoPath)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if len(state.Devices) == 0 {
		fmt.Fprintln(os.Stdout, "No devices registered.")
		return nil
	}

	fmt.Fprintf(os.Stdout, "%-36s  %-20s  %-12s  %-10s  %s\n", "ID", "NAME", "PLATFORM", "OS", "LAST SYNC")
	for _, d := range state.Devices {
		lastSync := "never"
		if !d.LastSync.IsZero() {
			lastSync = d.LastSync.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(os.Stdout, "%-36s  %-20s  %-12s  %-10s  %s\n", d.ID, d.Name, d.Platform, d.OS, lastSync)
	}

	return nil
}

// Rename renames a registered device.
func (c *DeviceCommand) Rename(repoPath, oldName, newName string) error {
	state, err := c.stateRepo.Load(repoPath)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	device := state.FindDevice(oldName)
	if device == nil {
		return fmt.Errorf("device '%s' not found", oldName)
	}

	device.Name = newName

	if err = c.stateRepo.Save(repoPath, state); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Renamed device '%s' to '%s'\n", oldName, newName)
	return nil
}

// Remove removes a registered device from the state.
func (c *DeviceCommand) Remove(repoPath, name string) error {
	state, err := c.stateRepo.Load(repoPath)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	found := false
	newDevices := state.Devices[:0]
	for _, d := range state.Devices {
		if d.Name == name {
			found = true
			continue
		}
		newDevices = append(newDevices, d)
	}

	if !found {
		return fmt.Errorf("device '%s' not found", name)
	}

	state.Devices = newDevices

	if err = c.stateRepo.Save(repoPath, state); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Removed device '%s'\n", name)
	return nil
}
