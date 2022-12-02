package dynamicupdate

// Ready implements the Ready interface.
func (d *DynamicUpdate) Ready() bool {
	for {
		select {
		case <-d.mgr.Elected():
			log.Infof("Elected as leader")
			return true
		default:
			return false
		}
	}
}
