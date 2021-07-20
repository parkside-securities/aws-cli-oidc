package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/defaults"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const expiryDelta = 10 * time.Second

type AWSCredentials struct {
	AWSAccessKey     string
	AWSSecretKey     string
	AWSSessionToken  string
	AWSSecurityToken string
	PrincipalARN     string
	Expires          time.Time
}

func (cred AWSCredentials) Valid() bool {
	if cred.Expires.IsZero() {
		return false
	}
	return !cred.Expires.Add(-expiryDelta).Before(time.Now())
}

func GetCredentialsWithOIDC(client *OIDCClient, idToken string, roleARN string, durationSeconds int64) (*AWSCredentials, error) {
	//role := strings.SplitN(roleARN, "/", 2)[1]
	var awsCredsBag AWSCredentials
	jsonString, err := getAWSTokenCache(roleARN)
	if err != nil {
		if err != ErrNotFound {
			return nil, err
		}
	} else {
		if err := json.Unmarshal([]byte(jsonString), &awsCredsBag); err != nil {
			return nil, err
		}
	}

	awsCreds := awsCredsBag
	if awsCreds.Valid() {
		log.Println("Retrieve active valid AWS STS credentials from cache")
		return &awsCreds, nil
	}

	token, err := assumeRoleWithWebIdentity(client, idToken, roleARN, durationSeconds)
	if err != nil {
		return nil, err
	}

	awsCredsBag = *token
	awsCredsBagJSON, err := json.Marshal(awsCredsBag)
	if err != nil {
		return nil, err
	}

	if err := saveAWSTokenCache(string(awsCredsBagJSON), roleARN); err != nil {
		return nil, err
	}

	return token, err
}

func assumeRoleWithWebIdentity(client *OIDCClient, idToken string, roleARN string, durationSeconds int64) (*AWSCredentials, error) {
	var username string
	if strings.Contains(os.Getenv("USER"), "\\") {
		username = strings.ToUpper(strings.SplitN(os.Getenv("USER"), "\\", 2)[1])
	} else {
		username = os.Getenv("USER")
	}
	split := strings.SplitN(roleARN, "/", 2)
	rolename := client.name
	if len(split) == 2 {
		rolename = split[1]
	}

	log.Printf("Requesting AWS STS credentials (%d seconds) using OIDC Token\n", durationSeconds)

	cfg := defaults.Config()
	cfg.Region = client.config.Region
	req := sts.New(cfg).AssumeRoleWithWebIdentityRequest(&sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(roleARN),
		RoleSessionName:  aws.String(username + "@" + rolename),
		WebIdentityToken: aws.String(idToken),
		DurationSeconds:  aws.Int64(durationSeconds),
	})
	resp, err := req.Send(context.Background())
	if err != nil {
		return nil, fmt.Errorf("error retrieving STS credentials using ID Token: %v", err)
	}

	return &AWSCredentials{
		AWSAccessKey:     aws.StringValue(resp.Credentials.AccessKeyId),
		AWSSecretKey:     aws.StringValue(resp.Credentials.SecretAccessKey),
		AWSSessionToken:  aws.StringValue(resp.Credentials.SessionToken),
		AWSSecurityToken: aws.StringValue(resp.Credentials.SessionToken),
		PrincipalARN:     aws.StringValue(resp.AssumedRoleUser.Arn),
		Expires:          resp.Credentials.Expiration.Local(),
	}, nil
}
