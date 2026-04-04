package entities

// Conflict represents a divergent change on a personal file between two devices.
type Conflict struct {
	Path          string
	LocalDevice   string
	RemoteDevice  string
	LocalContent  []byte
	RemoteContent []byte
}

// ConflictFileName returns the name for the conflict file.
func (c *Conflict) ConflictFileName() string {
	return c.Path + ".conflict." + c.RemoteDevice
}
