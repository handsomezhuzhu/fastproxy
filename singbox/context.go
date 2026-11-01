package singbox

import (
	"context"

	"github.com/sagernet/sing-box"
)

// context installs the manager's type registries into a sing-box context,
// exactly as box.Context does, so box.New and the option decoder agree on the
// set of supported inbound/outbound/DNS types.
func (r Registries) context(ctx context.Context) context.Context {
	return box.Context(ctx, r.Inbounds, r.Outbounds, r.Endpoints, r.DNS, r.Services, r.Certificates)
}
