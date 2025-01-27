package common

import (
	"net"
	"testing"
	"time"

	pc "github.com/opiproject/opi-api/network/opinetcommon/v1alpha1/gen/go"
	"github.com/stretchr/testify/assert"
)

func TestCheckReplayThreshold(t *testing.T) {
	component := &Component{
		Timer: 5 * time.Second,
	}

	// Case 1: Replay should be false if Timer <= Threshold
	component.CheckReplayThreshold(10 * time.Second)
	assert.False(t, component.Replay, "Expected Replay to be false when threshold is greater.")

	// Case 2: Replay should be true if Timer > Threshold
	component.CheckReplayThreshold(3 * time.Second)
	assert.True(t, component.Replay, "Expected Replay to be true when threshold is exceeded.")
}

func TestConvertToIPPrefix_ValidIPv4(t *testing.T) {
	_, ipNet, _ := net.ParseCIDR("192.168.1.0/24")

	result := ConvertToIPPrefix(ipNet)

	assert.NotNil(t, result, "Expected non-nil result")
	assert.Equal(t, pc.IpAf_IP_AF_INET, result.Addr.Af, "Expected IPv4 address family")
	assert.Equal(t, int32(24), result.Len, "Expected prefix length of 24")
	assert.Equal(t, uint32(192)<<24|uint32(168)<<16|uint32(1)<<8|uint32(0), result.Addr.GetV4Addr(), "Expected correct V4 address conversion")
}

func TestConvertToIPPrefix_NilInput(t *testing.T) {
	result := ConvertToIPPrefix(nil)
	assert.Nil(t, result, "Expected nil result for nil input")
}

func TestConvertToIPPrefix_ValidIPv6(t *testing.T) {
	_, ipNet, _ := net.ParseCIDR("2001:db8::/64")

	result := ConvertToIPPrefix(ipNet)

	assert.NotNil(t, result, "Expected non-nil result")
	assert.Equal(t, pc.IpAf_IP_AF_INET, result.Addr.Af, "Expected IPv4 family for V4 input")
	assert.Equal(t, int32(64), result.Len, "Expected prefix length of 64")
}
