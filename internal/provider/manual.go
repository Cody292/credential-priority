package provider

import "credential-priority/internal/core"

type manualStrategy struct {
	provider core.Provider
}

func (s manualStrategy) evaluate(credential core.Credential) Result {
	safeCredential := credential.WithProbe(core.FreshnessUnknown, core.ProbeStatusUnsupported)
	return Result{
		Credential:   safeCredential,
		Provider:     s.provider,
		StrategyName: core.StrategyManual,
		Freshness:    safeCredential.Freshness,
		ProbeStatus:  safeCredential.ProbeStatus,
		CanPromote:   core.CannotPromote,
	}
}
