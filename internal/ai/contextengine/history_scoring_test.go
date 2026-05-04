package contextengine

import (
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
)

func TestHistoryRecencyBoost(t *testing.T) {
	total := 10

	recent := historyRecencyBoost(9, total)
	old := historyRecencyBoost(0, total)
	assert.Greater(t, recent, old, "recent message should have higher boost than old")

	same := historyRecencyBoost(5, total)
	assert.Greater(t, recent, same, "most recent should have highest boost")
}

func TestHistoryRecencyBoost_SingleElement(t *testing.T) {
	boost := historyRecencyBoost(0, 1)
	assert.Equal(t, 0.0, boost)
}

func TestHistoryRoleBoost_User(t *testing.T) {
	msg := &schema.Message{Role: schema.User}
	boost := historyRoleBoost(msg)
	assert.InDelta(t, roleUserBonus, boost, 0.001)
}

func TestHistoryRoleBoost_Assistant(t *testing.T) {
	msg := &schema.Message{Role: schema.Assistant}
	boost := historyRoleBoost(msg)
	assert.Equal(t, 0.0, boost)
}

func TestHistoryEntityBoost_ServiceMatch(t *testing.T) {
	msg := &schema.Message{Content: "checkoutservice CPU 很高"}
	boost := historyEntityBoost(msg, "checkoutservice 怎么了")
	assert.InDelta(t, entityMatchBonus, boost, 0.001)
}

func TestHistoryEntityBoost_NoMatch(t *testing.T) {
	msg := &schema.Message{Content: "CPU usage normal"}
	boost := historyEntityBoost(msg, "Redis 超时")
	assert.Equal(t, 0.0, boost)
}

func TestHistoryEntityBoost_EntityMatch(t *testing.T) {
	msg := &schema.Message{Content: "Redis connection pool exhausted"}
	boost := historyEntityBoost(msg, "Redis 连接超时")
	assert.InDelta(t, entityMatchBonus, boost, 0.001)
}

func TestCombinedHistoryScore_WithBoosts(t *testing.T) {
	msg := &schema.Message{Role: schema.User, Content: "checkoutservice 故障"}
	query := "checkoutservice CPU 告警"

	cosine := 0.8
	combined := combinedHistoryScore(cosine, 9, 10, msg, query)

	assert.Greater(t, combined, cosine, "combined score should be higher than raw cosine")
}

func TestCombinedHistoryScore_NoBoosts(t *testing.T) {
	msg := &schema.Message{Role: schema.Assistant, Content: "unrelated content"}
	query := "Redis 超时"

	cosine := 0.8
	combined := combinedHistoryScore(cosine, 0, 1, msg, query)

	assert.InDelta(t, cosine, combined, 0.001, "score with no boosts should equal raw cosine * 1.0")
}
