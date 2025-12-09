package datalogger

type RemoteClient struct {
	BaseLogger
}

func NewRemote(cfg Config, lw LogWriter) (IClient, error) {
	return &RemoteClient{BaseLogger: NewBaseLogger(cfg, lw)}, nil
}

func (c *RemoteClient) Start() error {
	defer c.secondTicker.Stop()
	defer c.lw.Close()

	return nil
}
