package mobile

import "github.com/doug/gophics/shell"

// Links makes the Bridge a shell.LinksWindow. It returns nil today, leaving
// ctx.Links() nil so callers treat the app as having no launch/deep-link URL.
//
// The existing opened-URL plumbing on the Bridge (OpenURL/TakeOpenedURL,
// mobile.go) is the *outbound* drain: Go enqueues URLs for the host to open in
// the system browser. It is the wrong direction for this capability, which needs
// *inbound* URLs the host hands to Go, so it is not reused here.
//
// TODO(android/ios deep links): add the inbound counterpart to the outbound
// drain — a host-called Bridge method (e.g. DeliverURL(string), the mirror of
// TakeOpenedURL) that the host invokes from onCreate/onNewIntent (Android
// Intent data) and application(_:open:) / continue userActivity (iOS custom
// scheme + universal links). Store the launch URL for Initial() and fan
// in-session deliveries out through OnLink.
func (b *Bridge) Links() shell.Links { return nil }
