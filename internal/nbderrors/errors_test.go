package nbderrors

import "testing"

func TestErrorShape(t *testing.T) {
	err := New(CodePolicyRedisSelectRejected, "Redis SELECT is rejected").WithDatasource("saas-auth-support")
	if err.Code != CodePolicyRedisSelectRejected {
		t.Fatalf("code=%s", err.Code)
	}
	if err.Category != CategoryPolicy {
		t.Fatalf("category=%s", err.Category)
	}
	if err.Datasource != "saas-auth-support" {
		t.Fatalf("datasource=%s", err.Datasource)
	}
	if err.Retryable {
		t.Fatalf("policy errors must not be retryable")
	}
}
