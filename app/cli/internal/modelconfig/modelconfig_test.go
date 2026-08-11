package modelconfig

import "testing"

func TestRoleAndProviderChangesHaveExplicitSemantics(t *testing.T) {
	if err := (Role{Kind: UtilityRole}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Role{Kind: EmbeddingRole, Provider: "deepseek"}).Validate(); err == nil {
		t.Fatal("half-configured role was accepted")
	}
	secret := ValueChange{Kind: SetValue, Value: "secret"}
	update := UpdateProvider{Provider: "deepseek", APIKey: &secret}
	if err := update.Validate(); err != nil {
		t.Fatal(err)
	}
	secret.Value = ""
	if err := update.Validate(); err == nil {
		t.Fatal("empty key update was accepted")
	}
}
