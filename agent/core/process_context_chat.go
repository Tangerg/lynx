package core

import (
	"errors"
)

// Chat returns provider-neutral model/stream capabilities already scoped by
// runtime composition to this process's session and middleware policy.
func (pc *ProcessContext) Chat() (ChatCapability, error) {
	if pc == nil || pc.chat == nil {
		if pc != nil && pc.parallelBranch {
			return ChatCapability{}, ErrParallelBranchControl
		}
		return ChatCapability{}, errors.New("agent.ProcessContext.Chat: no chat model configured on the engine")
	}
	capability, err := pc.chat()
	if err != nil {
		return ChatCapability{}, err
	}
	if capability.Model == nil {
		return ChatCapability{}, errors.New("agent.ProcessContext.Chat: runtime resolved a nil chat model")
	}
	return capability, nil
}
