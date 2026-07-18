// Package version holds the build version stamped in via -ldflags:
//
//	go build -ldflags "-X github.com/fox27374/net-lama/internal/version.Version=git-abc1234"
package version

// Version is "dev" for plain go builds; Makefile and Containerfile stamp
// the git describe / short-sha value.
var Version = "dev"
