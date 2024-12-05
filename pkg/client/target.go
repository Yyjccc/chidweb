package client

import (
	"chidweb/pkg/common"
	"strings"
)

var (
	targetManger *TargetManger
)

type Target struct {
	ID   uint32
	addr TCPAddress
}

func NewTarget(tcpAddr string) *Target {
	split := strings.Split(tcpAddr, ":")
	addr := TCPAddress{
		Host: split[0],
		Port: split[1],
		Raw:  tcpAddr,
	}
	target := &Target{
		ID:   common.Generate32ID(),
		addr: addr,
	}
	manger := GetTargetManger()
	manger.AddTarget(target)
	return target

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

func (t *TargetManger) AddTarget(target *Target) {
	targetManger.targets[target.ID] = target
}

func GetTargetManger() *TargetManger {
	if targetManger == nil {
		targetManger = &TargetManger{
			targets: make(map[uint32]*Target),
		}
	}
	return targetManger
}
