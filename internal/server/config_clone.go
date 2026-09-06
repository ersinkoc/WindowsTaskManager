package server

import "github.com/ersinkoc/WindowsTaskManager/internal/config"

func cloneConfig(src *config.Config) config.Config {
	if src == nil {
		return *config.DefaultConfig()
	}
	dst := *src
	dst.Controller.ProtectedProcesses = append([]string(nil), src.Controller.ProtectedProcesses...)
	dst.Anomaly.IgnoreProcesses = append([]string(nil), src.Anomaly.IgnoreProcesses...)
	dst.Anomaly.HungProcess.IdleWhitelist = append([]string(nil), src.Anomaly.HungProcess.IdleWhitelist...)
	dst.Anomaly.RunawayCPU.HighCPUWhitelist = append([]string(nil), src.Anomaly.RunawayCPU.HighCPUWhitelist...)
	dst.Anomaly.NewProcess.SuspiciousPaths = append([]string(nil), src.Anomaly.NewProcess.SuspiciousPaths...)
	dst.WellKnownPorts = make(map[uint16]string, len(src.WellKnownPorts))
	for k, v := range src.WellKnownPorts {
		dst.WellKnownPorts[k] = v
	}
	dst.AI.ExtraHeaders = make(map[string]string, len(src.AI.ExtraHeaders))
	for k, v := range src.AI.ExtraHeaders {
		dst.AI.ExtraHeaders[k] = v
	}
	dst.AI.AutoAction.AllowedActions = append([]string(nil), src.AI.AutoAction.AllowedActions...)
	dst.Telegram.AllowedChatIDs = append([]int64(nil), src.Telegram.AllowedChatIDs...)
	dst.Telegram.NotificationTypes = append([]string(nil), src.Telegram.NotificationTypes...)
	dst.Rules = append([]config.Rule(nil), src.Rules...)
	return dst
}
