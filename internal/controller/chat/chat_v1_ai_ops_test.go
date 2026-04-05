package chat

import (
	"context"
	"testing"

	v1 "SuperBizAgent/api/chat/v1"
)

func TestAIOpsUsesDetailWhenResultIsEmpty(t *testing.T) {
	oldRun := runAIOpsMultiAgent
	defer func() {
		runAIOpsMultiAgent = oldRun
	}()

	runAIOpsMultiAgent = func(ctx context.Context, query string) (string, []string, string, error) {
		return "", []string{"审批拒绝：该请求需要人工确认"}, "", nil
	}

	ctrl := &ControllerV1{}
	res, err := ctrl.AIOps(context.Background(), &v1.AIOpsReq{Query: "请删除生产数据"})
	if err != nil {
		t.Fatalf("ai ops returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected response")
	}
	if res.Result != "审批拒绝：该请求需要人工确认" {
		t.Fatalf("unexpected result: %q", res.Result)
	}
	if len(res.Detail) != 1 || res.Detail[0] != res.Result {
		t.Fatalf("unexpected detail: %v", res.Detail)
	}
}

func TestAIOpsReturnsInternalErrorWhenResultAndDetailAreEmpty(t *testing.T) {
	oldRun := runAIOpsMultiAgent
	defer func() {
		runAIOpsMultiAgent = oldRun
	}()

	runAIOpsMultiAgent = func(ctx context.Context, query string) (string, []string, string, error) {
		return "", nil, "", nil
	}

	ctrl := &ControllerV1{}
	res, err := ctrl.AIOps(context.Background(), &v1.AIOpsReq{Query: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "内部错误" {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil response, got %#v", res)
	}
}
