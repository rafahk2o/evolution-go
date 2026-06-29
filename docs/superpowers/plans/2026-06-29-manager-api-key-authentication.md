# Manager API Key Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make global and company API keys reliably authenticate through the Manager after logout, while separating invalid credentials from backend failures.

**Architecture:** Canonicalize keys at the Manager and HTTP middleware boundaries. Keep global authorization explicit, use a read-only default-company lookup, and retain hashed active-company authentication. Return 401 only for missing or unknown keys and 500 for repository failures.

**Tech Stack:** Go 1.25, Gin, GORM, Node.js test runner, persisted Manager bundle.

---

## File Structure

- `pkg/company/repository/company_repository.go`: read-only default-company lookup.
- `pkg/company/repository/company_repository_test.go`: lookup regression test.
- `pkg/company/service/company_service.go`: expose the lookup to middleware.
- `pkg/middleware/auth_middleware.go`: normalize and classify authentication.
- `pkg/middleware/auth_middleware_test.go`: backend regression coverage.
- `manager/dist/assets/index-Dx4-byTC.js`: normalize Manager login values.
- `manager/dist/assets/auth-session.test.js`: bundle regression coverage.

### Task 1: Read-Only Default Company Lookup

**Files:**
- Modify: `pkg/company/repository/company_repository.go`
- Modify: `pkg/company/repository/company_repository_test.go`
- Modify: `pkg/company/service/company_service.go`

- [ ] **Step 1: Write the failing repository test**

Append to `pkg/company/repository/company_repository_test.go`:

```go
func TestGetDefaultCompanyUsesReadOnlyActiveDefaultLookup(t *testing.T) {
	capture := &queryCaptureLogger{}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: dryRunConnPool{}, WithoutReturning: true,
	}), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, Logger: capture,
	})
	if err != nil {
		t.Fatalf("create dry-run database: %v", err)
	}

	repository := NewCompanyRepository(db)
	_, _ = repository.GetDefaultCompany()
	statement := strings.Join(capture.statements, "\n")
	if !strings.Contains(statement, `"name" = 'default'`) {
		t.Fatalf("expected default company filter, got SQL: %s", statement)
	}
	if !strings.Contains(statement, `"active" = true`) {
		t.Fatalf("expected active company filter, got SQL: %s", statement)
	}
	if strings.Contains(strings.ToUpper(statement), "UPDATE") {
		t.Fatalf("default lookup must be read-only, got SQL: %s", statement)
	}
}
```

- [ ] **Step 2: Run the test and verify RED**

Run: `go test ./pkg/company/repository -run TestGetDefaultCompanyUsesReadOnlyActiveDefaultLookup -v`

Expected: compilation fails because `GetDefaultCompany` does not exist.

- [ ] **Step 3: Implement the repository method**

Add to `CompanyRepository` and implement:

```go
GetDefaultCompany() (*company_model.Company, error)

func (r *companyRepository) GetDefaultCompany() (*company_model.Company, error) {
	var company company_model.Company
	if err := r.db.Where("name = ? AND active = ?", DefaultCompanyName, true).
		First(&company).Error; err != nil {
		return nil, err
	}
	return &company, nil
}
```

- [ ] **Step 4: Expose it through the company service**

Add to `CompanyService` and implement:

```go
GetDefaultCompany() (*company_model.Company, error)

func (c *companies) GetDefaultCompany() (*company_model.Company, error) {
	return c.companyRepository.GetDefaultCompany()
}
```

- [ ] **Step 5: Format and verify GREEN**

Run: `gofmt -w pkg/company/repository/company_repository.go pkg/company/repository/company_repository_test.go pkg/company/service/company_service.go`

Run: `go test ./pkg/company/...`

Expected: all company package tests pass.

- [ ] **Step 6: Commit**

```powershell
git add pkg/company/repository/company_repository.go pkg/company/repository/company_repository_test.go pkg/company/service/company_service.go
git commit -m "fix: make default company auth lookup read only"
```

### Task 2: Canonical Backend Authentication

**Files:**
- Modify: `pkg/middleware/auth_middleware_test.go`
- Modify: `pkg/middleware/auth_middleware.go`

- [ ] **Step 1: Extend the middleware fake**

Add fields and the new method to `authCompanyService`; keep its existing no-op
interface methods:

```go
type authCompanyService struct {
	ensuredKey       string
	authenticatedKey string
	authenticated    *company_model.Company
	authenticateErr  error
	defaultCompany   *company_model.Company
	defaultErr       error
}

func (s *authCompanyService) AuthenticateAPIKey(apiKey string) (*company_model.Company, error) {
	s.authenticatedKey = apiKey
	if s.authenticateErr != nil { return nil, s.authenticateErr }
	if s.authenticated != nil { return s.authenticated, nil }
	return &company_model.Company{Id: "tenant-company"}, nil
}

func (s *authCompanyService) GetDefaultCompany() (*company_model.Company, error) {
	if s.defaultErr != nil { return nil, s.defaultErr }
	if s.defaultCompany != nil { return s.defaultCompany, nil }
	return &company_model.Company{Id: "default-company"}, nil
}
```

Add imports for `errors` and `gorm.io/gorm`.

- [ ] **Step 2: Replace the global regression test**

```go
func TestAuthAdminNormalizesGlobalAPIKeyAndUsesDefaultCompany(t *testing.T) {
	gin.SetMode(gin.TestMode)
	companies := &authCompanyService{}
	middleware := middleware{
		config: &config.Config{GlobalApiKey: "global-secret"},
		companyService: companies,
	}
	router := gin.New()
	router.GET("/instance/all", middleware.AuthAdmin, func(ctx *gin.Context) {
		companyID, _ := ctx.Get("companyId")
		isMaster, _ := ctx.Get("isMaster")
		if companyID != "default-company" || isMaster != true {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		ctx.Status(http.StatusOK)
	})
	request := httptest.NewRequest(http.MethodGet, "/instance/all", nil)
	request.Header.Set("apikey", "  global-secret  ")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK { t.Fatalf("unexpected status: %d", response.Code) }
	if companies.ensuredKey != "" { t.Fatal("request authentication synchronized default company") }
	if companies.authenticatedKey != "" { t.Fatalf("global key used tenant auth: %q", companies.authenticatedKey) }
}
```

- [ ] **Step 3: Add the company normalization test**

```go
func TestAuthAdminNormalizesCompanyAPIKey(t *testing.T) {
	companies := &authCompanyService{authenticated: &company_model.Company{Id: "tenant-company"}}
	middleware := middleware{
		config: &config.Config{GlobalApiKey: "global-secret"},
		companyService: companies,
	}
	router := gin.New()
	router.GET("/instance/all", middleware.AuthAdmin, func(ctx *gin.Context) {
		companyID, _ := ctx.Get("companyId")
		if companyID != "tenant-company" {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		ctx.Status(http.StatusOK)
	})
	request := httptest.NewRequest(http.MethodGet, "/instance/all", nil)
	request.Header.Set("apikey", "  evo_co_company-secret  ")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK { t.Fatalf("unexpected status: %d", response.Code) }
	if companies.authenticatedKey != "evo_co_company-secret" {
		t.Fatalf("company key was not normalized: %q", companies.authenticatedKey)
	}
}
```

- [ ] **Step 4: Add error classification tests**

Use this table test:

```go
func TestAuthAdminClassifiesAuthenticationErrors(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		companies  *authCompanyService
		wantStatus int
	}{
		{"unknown company key", "unknown", &authCompanyService{authenticateErr: gorm.ErrRecordNotFound}, http.StatusUnauthorized},
		{"company database failure", "company-key", &authCompanyService{authenticateErr: errors.New("database unavailable")}, http.StatusInternalServerError},
		{"default database failure", "global-secret", &authCompanyService{defaultErr: errors.New("database unavailable")}, http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			middleware := middleware{
				config: &config.Config{GlobalApiKey: "global-secret"},
				companyService: test.companies,
			}
			router := gin.New()
			router.GET("/instance/all", middleware.AuthAdmin)
			request := httptest.NewRequest(http.MethodGet, "/instance/all", nil)
			request.Header.Set("apikey", test.key)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("expected %d, got %d", test.wantStatus, response.Code)
			}
		})
	}
}
```

Add the master-route normalization regression:

```go
func TestAuthMasterNormalizesGlobalAPIKey(t *testing.T) {
	middleware := middleware{config: &config.Config{GlobalApiKey: "global-secret"}}
	router := gin.New()
	router.GET("/company/all", middleware.AuthMaster, func(ctx *gin.Context) {
		ctx.Status(http.StatusOK)
	})
	request := httptest.NewRequest(http.MethodGet, "/company/all", nil)
	request.Header.Set("apikey", "  global-secret  ")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
}
```

- [ ] **Step 5: Run middleware tests and verify RED**

Run: `go test ./pkg/middleware -run TestAuthAdmin -v`

Expected: whitespace, read-only lookup, and 500-status cases fail.

- [ ] **Step 6: Implement canonical authentication**

Add `errors`, `log`, `strings`, and `gorm.io/gorm` imports. Replace `AuthAdmin`:

```go
func (m middleware) AuthAdmin(ctx *gin.Context) {
	token := strings.TrimSpace(ctx.GetHeader("apikey"))
	if token == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}
	if token == strings.TrimSpace(m.config.GlobalApiKey) {
		if companyID := strings.TrimSpace(ctx.GetHeader("X-Company-Id")); companyID != "" {
			ctx.Set("companyId", companyID)
			ctx.Set("isMaster", true)
			ctx.Next()
			return
		}
		company, err := m.companyService.GetDefaultCompany()
		if err != nil {
			log.Printf("default company authentication lookup failed: %v", err)
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		ctx.Set("company", company)
		ctx.Set("companyId", company.Id)
		ctx.Set("isMaster", true)
		ctx.Next()
		return
	}
	company, err := m.companyService.AuthenticateAPIKey(token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
			return
		}
		log.Printf("company API key authentication lookup failed: %v", err)
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	ctx.Set("company", company)
	ctx.Set("companyId", company.Id)
	ctx.Next()
}
```

Also trim `ctx.GetHeader("apikey")` and `m.config.GlobalApiKey` in `AuthMaster`.

- [ ] **Step 7: Format, verify GREEN, and commit**

Run: `gofmt -w pkg/middleware/auth_middleware.go pkg/middleware/auth_middleware_test.go`

Run: `go test ./pkg/middleware ./pkg/company/...`

Expected: all focused tests pass.

```powershell
git add pkg/middleware/auth_middleware.go pkg/middleware/auth_middleware_test.go
git commit -m "fix: normalize manager api key authentication"
```

### Task 3: Normalize Manager Login Credentials

**Files:**
- Modify: `manager/dist/assets/auth-session.test.js`
- Modify: `manager/dist/assets/index-Dx4-byTC.js`

- [ ] **Step 1: Add failing bundle tests**

Append:

```javascript
test("manager normalizes API keys before authentication", () => {
  const source = managerBundle();
  assert.match(source, /apiKey\.trim\(\)/);
  assert.match(source, /login:async\([^)]*\)=>\{[^}]*\.trim\(\)/);
});

test("manager login reads normalized interceptor status", () => {
  const source = managerBundle();
  const start = source.indexOf('console.error("Login error:",');
  assert.notEqual(start, -1, "login error handler not found");
  assert.match(source.slice(start, start + 400), /c==null\?void 0:c\.status/);
});

test("logout remains a client-only operation", () => {
  const source = managerBundle();
  const start = source.indexOf('logout:()=>');
  assert.notEqual(start, -1, "logout handler not found");
  const logout = source.slice(start, start + 350);
  assert.match(logout, /localStorage\.removeItem\("evolution-auth"\)/);
  assert.doesNotMatch(logout, /Xt\.(post|put|patch|delete)/);
});
```

- [ ] **Step 2: Run tests and verify RED**

Run: `node --test manager/dist/assets/auth-session.test.js`

Expected: the normalization and interceptor-status tests fail; the logout
contract test passes because logout already changes only client state.

- [ ] **Step 3: Normalize the store login key**

Change the minified login fragment to use `o=r.trim()`:

```javascript
login:async(n,r)=>{var i;const s=n.replace(/\/$/,""),o=r.trim();try{await Xt.get("/instance/all",{baseURL:s,headers:{apikey:o,"Cache-Control":"no-cache"},params:{t:Date.now()}}),t({apiUrl:s,apiKey:o,isAuthenticated:!0})}
```

Read either normalized interceptor status or raw Axios status:

```javascript
const u=(c==null?void 0:c.status)??((i=c==null?void 0:c.response)==null?void 0:i.status);
```

- [ ] **Step 4: Normalize the form submission**

In the login form handler introduce:

```javascript
const j=A.apiUrl.replace(/\/$/,""),R=A.apiKey.trim();
```

Use `R` in `await n(j,R)`, `s(R)`, and `await t(A.apiUrl,R)`.

- [ ] **Step 5: Verify GREEN and commit**

Run: `node --test manager/dist/assets/auth-session.test.js`

Expected: all Manager authentication tests pass.

```powershell
git add manager/dist/assets/index-Dx4-byTC.js manager/dist/assets/auth-session.test.js
git commit -m "fix: normalize api keys on manager login"
```

### Task 4: Full Verification

**Files:** Verify only.

- [ ] **Step 1: Run focused regressions**

Run: `go test ./pkg/middleware ./pkg/company/...`

Run: `node --test manager/dist/assets/auth-session.test.js`

Expected: all focused tests pass.

- [ ] **Step 2: Run the full Go suite**

Run: `go test ./...`

Expected: all packages pass. Record exact environment-dependent skips or failures.

- [ ] **Step 3: Check the worktree**

Run: `git diff --check`

Run: `git status --short`

Expected: no whitespace errors and only intentional uncommitted files, if any.

- [ ] **Step 4: Scan for accidental credentials**

Run:

```powershell
git diff HEAD~3 -- . ':!docs/superpowers/specs/*' ':!docs/superpowers/plans/*' | Select-String -Pattern 'GLOBAL_API_KEY=|evo_co_[0-9a-f]{20,}'
```

Expected: no real credentials; synthetic test values are acceptable.

- [ ] **Step 5: Record deployment requirements**

The handoff must state that the Go binary and `manager/dist` need to be rebuilt
and deployed together. Because the asset filename remains unchanged, invalidate
the browser/proxy cache or perform a hard refresh after deployment.
