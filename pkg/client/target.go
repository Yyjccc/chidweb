package client

type Target struct {
	ID   uint32
	addr TCPAddress
}

type TargetManger struct {
	targets map[uint32]*Target
}

func (t *TargetManger) GetNoConnectTarget() *Target {
	return &Target{
		ID:   0,
		addr: TCPAddress{},
	}

}

func (t *TargetManger) GetTarget(id uint32) *Target {
	return t.targets[id]

}
