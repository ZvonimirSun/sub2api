//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRegistrationEmailSuffixWhitelist(t *testing.T) {
	got, err := NormalizeRegistrationEmailSuffixWhitelist([]string{"example.com", "@EXAMPLE.COM", " @foo.bar ", "*.EDU.CN"})
	require.NoError(t, err)
	require.Equal(t, []string{"@example.com", "@foo.bar", "*.edu.cn"}, got)
}

func TestNormalizeRegistrationEmailSuffixWhitelist_Invalid(t *testing.T) {
	for _, item := range []string{"@invalid_domain", "*.", "*", "*.@", "*.foo"} {
		t.Run(item, func(t *testing.T) {
			_, err := NormalizeRegistrationEmailSuffixWhitelist([]string{item})
			require.Error(t, err)
		})
	}
}

func TestParseRegistrationEmailSuffixWhitelist(t *testing.T) {
	got := ParseRegistrationEmailSuffixWhitelist(`["example.com","@foo.bar","*.EDU.CN","@invalid_domain","*.foo"]`)
	require.Equal(t, []string{"@example.com", "@foo.bar", "*.edu.cn"}, got)
}

func TestIsRegistrationEmailSuffixAllowed(t *testing.T) {
	require.True(t, IsRegistrationEmailSuffixAllowed("user@example.com", []string{"@example.com"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@example.com.", []string{"@example.com"}))
	require.False(t, IsRegistrationEmailSuffixAllowed("user@sub.example.com", []string{"@example.com"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@qq.com", []string{"@qq.com"}))
	require.False(t, IsRegistrationEmailSuffixAllowed("user@sub.qq.com", []string{"@qq.com"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("student@cs.edu.cn", []string{"*.edu.cn"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("student@edu.cn", []string{"*.edu.cn"}))
	require.False(t, IsRegistrationEmailSuffixAllowed("student@foo.cn", []string{"*.edu.cn"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@a.com", []string{"@a.com", "*.b.cn"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@school.b.cn", []string{"@a.com", "*.b.cn"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@b.cn", []string{"@a.com", "*.b.cn"}))
	require.False(t, IsRegistrationEmailSuffixAllowed("user@c.cn", []string{"@a.com", "*.b.cn"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@any.com", []string{}))
}

func TestIsQQEmailLocalPartAllowed(t *testing.T) {
	require.True(t, IsQQEmailLocalPartAllowed("123456@qq.com"))
	require.True(t, IsQQEmailLocalPartAllowed("123456@qq.com."))
	require.True(t, IsQQEmailLocalPartAllowed(" 123456@QQ.COM "))
	require.True(t, IsQQEmailLocalPartAllowed("user@example.com"))
	require.True(t, IsQQEmailLocalPartAllowed("user@qq.com.example.com"))
	require.False(t, IsQQEmailLocalPartAllowed("abc123@qq.com"))
	require.False(t, IsQQEmailLocalPartAllowed("name@qq.com"))
	require.False(t, IsQQEmailLocalPartAllowed("123+tag@qq.com"))
	require.False(t, IsQQEmailLocalPartAllowed("123.456@qq.com"))
	require.False(t, IsQQEmailLocalPartAllowed("name@QQ.COM."))
	require.False(t, IsQQEmailLocalPartAllowed("\uff11\uff12\uff13@qq.com"))
}

func TestQQNumericEmailPolicyCannotBeBypassedBySettings(t *testing.T) {
	ctx := context.Background()
	for _, whitelist := range []string{`[]`, `["@qq.com"]`, `["*.qq.com"]`} {
		t.Run(whitelist, func(t *testing.T) {
			repo := &userRepoStub{}
			svc := newAuthService(repo, map[string]string{
				SettingKeyRegistrationEnabled:                 "true",
				SettingKeyRegistrationEmailSuffixWhitelist:    whitelist,
				SettingKeyRegistrationEmailDomainQuotaEnabled: "true",
			}, nil, nil)
			_, _, err := svc.Register(ctx, "alias@qq.com", "password")
			require.ErrorIs(t, err, ErrEmailSuffixNotAllowed)
			require.ErrorIs(t, svc.SendVerifyCode(ctx, "alias@qq.com"), ErrEmailSuffixNotAllowed)
			_, err = svc.SendVerifyCodeAsync(ctx, "alias@qq.com")
			require.ErrorIs(t, err, ErrEmailSuffixNotAllowed)
			_, err = svc.createEmailOAuthUser(ctx, "alias@qq.com", "name", "oidc", "", "")
			require.ErrorIs(t, err, ErrEmailSuffixNotAllowed)
			_, _, err = svc.LoginOrRegisterOAuth(ctx, "alias@qq.com", "name")
			require.ErrorIs(t, err, ErrEmailSuffixNotAllowed)
			svc.refreshTokenCache = &refreshTokenCacheStub{}
			_, _, err = svc.LoginOrRegisterOAuthWithTokenPair(ctx, "alias@qq.com", "name", "", "", "oidc")
			require.ErrorIs(t, err, ErrEmailSuffixNotAllowed)
			require.Empty(t, repo.created)
		})
	}
	normalized, err := normalizeEmailForIdentityBinding(" 123456@QQ.COM ")
	require.NoError(t, err)
	require.Equal(t, "123456@qq.com", normalized)
}

func TestQQNumericEmailRegistrationAllowed(t *testing.T) {
	repo := &userRepoStub{nextID: 9}
	svc := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:              "true",
		SettingKeyRegistrationEmailSuffixWhitelist: `["@qq.com"]`,
	}, nil, nil)
	_, user, err := svc.Register(context.Background(), "123456@qq.com", "password")
	require.NoError(t, err)
	require.Equal(t, "123456@qq.com", user.Email)
}

func TestRegistrationEmailPolicyChecksWhitelistBeforeQQRule(t *testing.T) {
	svc := newAuthService(&userRepoStub{}, map[string]string{
		SettingKeyRegistrationEmailSuffixWhitelist: `["@example.com"]`,
	}, nil, nil)
	ctx := context.Background()
	for _, email := range []string{"alias@qq.com", "123456@qq.com"} {
		err := svc.validateRegistrationEmailPolicy(ctx, email)
		require.ErrorIs(t, err, ErrEmailSuffixNotAllowed)
		require.EqualError(t, err, buildEmailSuffixNotAllowedError([]string{"@example.com"}).Error())
	}
	require.NoError(t, svc.validateRegistrationEmailPolicy(ctx, "alias@example.com"))
}

func TestQQNumericPolicyDoesNotBlockExistingOAuthLogin(t *testing.T) {
	user := &User{ID: 9, Email: "alias@qq.com", Status: StatusActive}
	repo := &userRepoStub{user: user}
	svc := newAuthService(repo, nil, nil, nil)
	_, actual, err := svc.LoginOrRegisterOAuth(context.Background(), user.Email, "name")
	require.NoError(t, err)
	require.Equal(t, user.ID, actual.ID)
	require.Empty(t, repo.created)
}

func TestRegistrationEmailQuotaRejectsMalformedDomainWhenWhitelistConfigured(t *testing.T) {
	repo := &userRepoStub{}
	svc := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                 "true",
		SettingKeyRegistrationEmailSuffixWhitelist:    `["@example.com"]`,
		SettingKeyRegistrationEmailDomainQuotaEnabled: "true",
	}, nil, nil)

	_, _, err := svc.Register(context.Background(), "malformed-email", "password")

	require.ErrorIs(t, err, ErrEmailSuffixNotAllowed)
	require.Empty(t, repo.created)
}

func TestIsRegistrationEmailSuffixLimited(t *testing.T) {
	require.False(t, IsRegistrationEmailSuffixLimited("user@custom.example", nil))
	require.False(t, IsRegistrationEmailSuffixLimited("user@example.com", []string{"@example.com"}))
	require.True(t, IsRegistrationEmailSuffixLimited("user@custom.example", []string{"@example.com"}))
}

func TestRegistrationEmailDomainUsesRegistrableDomain(t *testing.T) {
	require.Equal(t, "abc.com", RegistrationEmailDomain("user@abc.com"))
	require.Equal(t, "abc.com", RegistrationEmailDomain("user@abcd.abc.com"))
	require.Equal(t, "example.co.uk", RegistrationEmailDomain("user@team.example.co.uk"))
	require.Equal(t, "example.com", RegistrationEmailDomain("user@example.com."))
	require.Equal(t, "example.com", RegistrationEmailDomain("user@team.example.com."))
}
