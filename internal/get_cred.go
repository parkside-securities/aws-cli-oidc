package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"time"

	"github.com/pkg/browser"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
)

type OIDCClient struct {
	name   string
	config *providerConfig
}

type oidcToken struct {
	*oauth2.Token
	IDToken string `json:"id_token,omitempty"`
}

type AWSCredentialsJSON struct {
	Version         int
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string
	SessionToken    string
}

func oidcTokenFromOAuth2Token(token *oauth2.Token) *oidcToken {
	oidcToken := &oidcToken{
		Token:   token,
		IDToken: token.Extra("id_token").(string),
	}
	return oidcToken
}

func (t oidcToken) OAuth2Token() *oauth2.Token {
	return t.WithExtra(map[string]interface{}{
		"id_token": t.IDToken,
	})
}

func GetCred(providerName string, roleARN string, printCred bool, expire int64) error {
	config, err := readProviderConfig(providerName)
	if err != nil {
		return fmt.Errorf("failed to login OIDC provider: %v", err)
	}
	client := &OIDCClient{providerName, config}
	//role := ARNtoShortName(roleARN)
	//role := roleARN
	tokenResponse, err := getOIDCToken(client, providerName)
	if err != nil {
		return fmt.Errorf("failed to login the OIDC provider: %v", err)
	}

	log.Println("OIDC Login successful!")

	var duration int64
	if expire > client.config.MaxSessionDurationSeconds || expire == 0 {
		duration = client.config.MaxSessionDurationSeconds
	} else {
		if expire < 900 {
			duration = 900
		} else {
			duration = expire
		}
	}
	awsCreds, err := GetCredentialsWithOIDC(client, tokenResponse.IDToken, roleARN, duration)
	if err != nil {
		return fmt.Errorf("unable to get AWS Credentials: %v", err)
	}

	if printCred {
		Writeln("")
		Export("AWS_ACCESS_KEY_ID", awsCreds.AWSAccessKey)
		Export("AWS_SECRET_ACCESS_KEY", awsCreds.AWSSecretKey)
		Export("AWS_SESSION_TOKEN", awsCreds.AWSSessionToken)
	} else {
		awsCredsJSON := AWSCredentialsJSON{
			Version:         1,
			AccessKeyID:     awsCreds.AWSAccessKey,
			SecretAccessKey: awsCreds.AWSSecretKey,
			SessionToken:    awsCreds.AWSSessionToken,
		}

		jsonBytes, err := json.Marshal(awsCredsJSON)
		if err != nil {
			return fmt.Errorf("error: %v", err)
		}
		fmt.Println(string(jsonBytes))
	}
	return nil
}

func ARNtoShortName(arn string) string {
	r := regexp.MustCompile("arn:aws:iam:.*:\\d+:role/(\\S+)")
	role := r.FindStringSubmatch(arn)[1]
	role0 := r.FindStringSubmatch(arn)[0]
	println("role:", role0, role)
	return role
}

func getOIDCToken(client *OIDCClient, providerName string) (*oidcToken, error) {
	conf := &oauth2.Config{
		ClientID:     client.config.ClientID,
		ClientSecret: client.config.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  client.config.AuthURL,
			TokenURL: client.config.TokenURL,
		},
		Scopes: []string{"openid", "email"},
	}

	var token *oauth2.Token
	writeBack := false

	var oidcToken *oidcToken = nil
	jsonRaw, err := getOIDCTokenCache(providerName)
	if err != nil {
		if err != ErrNotFound {
			return nil, err
		}
	} else {
		if err := json.Unmarshal([]byte(jsonRaw), &oidcToken); err != nil {
			return nil, err
		}
	}

	if oidcToken != nil { // cache hit
		token = oidcToken.OAuth2Token()

		if !token.Valid() {
			writeBack = true

			tokenSource := conf.TokenSource(context.Background(), token)
			token, err = tokenSource.Token()
			// If we get an error here, we assume that the refresh token expired. Since token remains nil, the next
			// step will trigger a login flow.
			_ = err
		}
	}

	if token == nil { // cache miss or expired refresh token
		writeBack = true

		token, err = doLogin(conf, providerName)
		if err != nil {
			return nil, err
		}
	}

	oidcToken = oidcTokenFromOAuth2Token(token)

	if writeBack {
		tokenJSON, _ := json.Marshal(oidcToken)
		if err := saveOIDCTokenCache(string(tokenJSON), providerName); err != nil {
			return nil, err
		}
	}

	return oidcToken, nil
}

func doLogin(conf *oauth2.Config, providerName string) (*oauth2.Token, error) {
	address := "localhost:52327"
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("cannot start local http server to handle login redirect: %v", err)
	}

	conf.RedirectURL = "http://" + address

	ctx := context.Background()
	roleOption := oauth2.SetAuthURLParam("role", providerName)
	url := conf.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.ApprovalForce, roleOption)
	println(url)

	code := launch(url, listener)

	tok, err := conf.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("error during token exchange: %v", err)
	}

	return tok, err
}

func launch(url string, listener net.Listener) string {
	c := make(chan string)

	http.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		url := req.URL
		q := url.Query()
		code := q.Get("code")

		res.Header().Set("Content-Type", "text/html")

		// Response result page
		message := "Login "
		if code != "" {
			message += "successful"
		} else {
			message += "failed"
		}
		res.Header().Set("Cache-Control", "no-store")
		res.Header().Set("Pragma", "no-cache")
		res.WriteHeader(200)
		_, _ = res.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<body>
%s
</body>
</html>
`, message)))

		if f, ok := res.(http.Flusher); ok {
			f.Flush()
		}

		c <- code
	})

	srv := &http.Server{}
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		_ = srv.Shutdown(ctx)
	}()

	go func() {
		_ = srv.Serve(listener)
	}()

	var code string
	if err := browser.OpenURL(url); err == nil {
		code = <-c
	}

	return code
}

func Writeln(format string, msg ...interface{}) {
	fmt.Fprintln(os.Stderr, fmt.Sprintf(format, msg...))
}

func Export(key string, value string) {
	var msg string
	if runtime.GOOS == "windows" {
		msg = fmt.Sprintf("set %s=%s\n", key, value)
	} else {
		msg = fmt.Sprintf("export %s=%s\n", key, value)
	}
	fmt.Fprint(os.Stdout, msg)
}
