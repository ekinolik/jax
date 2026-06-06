package signals

import (
	"github.com/ekinolik/jax/pkg/confluence"
)

// SharesPerContract matches standard US equity options (same as option service).
const SharesPerContract = 100

// StrikeGEX computes net gamma exposure at a strike using option.go sign convention.
func StrikeGEX(sp confluence.StrikeProfile, spot float64) float64 {
	call := float64(sp.CallOI) * SharesPerContract * float64(sp.CallGamma) * spot
	put := float64(sp.PutOI) * SharesPerContract * float64(sp.PutGamma) * spot
	return call + put
}

// StrikeDEX computes net delta exposure at a strike using option.go sign convention.
func StrikeDEX(sp confluence.StrikeProfile, spot float64) float64 {
	call := float64(sp.CallOI) * SharesPerContract * float64(sp.CallDelta) * spot
	put := float64(sp.PutOI) * SharesPerContract * float64(sp.PutDelta) * spot
	return call + put
}
