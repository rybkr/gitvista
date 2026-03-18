package hosted

import "testing"

func TestMemoryHostedStore_CreateAndListAccounts(t *testing.T) {
	store := NewMemoryHostedStore(newTestRepoManager(t))

	account, err := store.CreateAccount("Acme", "acme")
	if err != nil {
		t.Fatalf("CreateAccount returned error: %v", err)
	}
	if account.Slug != "acme" {
		t.Fatalf("slug = %q, want %q", account.Slug, "acme")
	}

	accounts, err := store.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts returned error: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("account count = %d, want 2", len(accounts))
	}
	if accounts[0].Slug != DefaultHostedAccountSlug {
		t.Fatalf("first account slug = %q, want %q", accounts[0].Slug, DefaultHostedAccountSlug)
	}
}
