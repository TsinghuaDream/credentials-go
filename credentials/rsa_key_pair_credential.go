package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aliyun/credentials-go/credentials/request"
	"github.com/aliyun/credentials-go/credentials/utils"
	jmespath "github.com/jmespath/go-jmespath"
)

type RsaKeyPairCredential struct {
	*credentialUpdater
	PrivateKey        string
	PublicKeyId       string
	SessionExpiration int
	sessionCredential *SessionCredential
	runtime           *utils.Runtime
}

func newRsaKeyPairCredential(privateKey, publicKeyId string, sessionExpiration int, runtime *utils.Runtime) *RsaKeyPairCredential {
	return &RsaKeyPairCredential{
		PrivateKey:        privateKey,
		PublicKeyId:       publicKeyId,
		SessionExpiration: sessionExpiration,
		credentialUpdater: new(credentialUpdater),
		runtime:           runtime,
	}
}

func (r *RsaKeyPairCredential) GetAccessKeyId() (string, error) {
	if r.sessionCredential == nil || r.needUpdateCredential() {
		err := r.UpdateCredential()
		if err != nil {
			return "", err
		}
	}
	return r.sessionCredential.AccessKeyId, nil
}

func (r *RsaKeyPairCredential) GetAccessSecret() (string, error) {
	if r.sessionCredential == nil || r.needUpdateCredential() {
		err := r.UpdateCredential()
		if err != nil {
			return "", err
		}
	}
	return r.sessionCredential.AccessKeySecret, nil
}

func (r *RsaKeyPairCredential) GetSecurityToken() (string, error) {
	return "", nil
}

func (r *RsaKeyPairCredential) GetBearerToken() string {
	return ""
}

func (r *RsaKeyPairCredential) GetType() string {
	return "rsa_key_pair"
}

func (r *RsaKeyPairCredential) UpdateCredential() (err error) {
	if r.runtime == nil {
		r.runtime = new(utils.Runtime)
	}
	request := request.NewCommonRequest()
	request.Domain = "sts.aliyuncs.com"
	if r.runtime.Host != "" {
		request.Domain = r.runtime.Host
	}
	request.Scheme = "HTTPS"
	request.Method = "GET"
	request.QueryParams["AccessKeyId"] = r.PublicKeyId
	request.QueryParams["Action"] = "GenerateSessionAccessKey"
	request.QueryParams["Format"] = "JSON"
	if r.SessionExpiration > 0 {
		if r.SessionExpiration >= 900 && r.SessionExpiration <= 3600 {
			request.QueryParams["DurationSeconds"] = strconv.Itoa(r.SessionExpiration)
		} else {
			err = errors.New("[InvalidParam]:Key Pair session duration should be in the range of 15min - 1Hr")
			return
		}
	} else {
		request.QueryParams["DurationSeconds"] = strconv.Itoa(defaultDurationSeconds)
	}
	request.QueryParams["SignatureMethod"] = "SHA256withRSA"
	request.QueryParams["SignatureType"] = "PRIVATEKEY"
	request.QueryParams["SignatureVersion"] = "1.0"
	request.QueryParams["Version"] = "2015-04-01"
	request.QueryParams["Timestamp"] = utils.GetTimeInFormatISO8601()
	request.QueryParams["SignatureNonce"] = utils.GetUUID()
	signature := utils.Sha256WithRsa(request.BuildStringToSign(), r.PrivateKey)
	request.QueryParams["Signature"] = signature
	request.Headers["Host"] = request.Domain
	request.Headers["Accept-Encoding"] = "identity"
	request.Url = request.BuildUrl()
	content, err := doAction(request, r.runtime)
	if err != nil {
		return fmt.Errorf("refresh KeyPair err: %s", err.Error())
	}
	var data interface{}
	err = json.Unmarshal(content, &data)
	if err != nil {
		return fmt.Errorf("refresh KeyPair err: Json.Unmarshal fail: %s", err.Error())
	}
	accessKeyId, err := jmespath.Search("SessionAccessKey.SessionAccessKeyId", data)
	if err != nil {
		return fmt.Errorf("refresh KeyPair err: Fail to get SessionAccessKeyId: %s", err.Error())
	}
	accessKeySecret, err := jmespath.Search("SessionAccessKey.SessionAccessKeySecret", data)
	if err != nil {
		return fmt.Errorf("refresh KeyPair err: Fail to get SessionAccessKeySecret: %s", err.Error())
	}
	expiration, err := jmespath.Search("SessionAccessKey.Expiration", data)
	if err != nil {
		return fmt.Errorf("refresh KeyPair err: Fail to get Expiration: %s", err.Error())
	}
	if accessKeyId == nil || accessKeySecret == nil || expiration == nil {
		return fmt.Errorf("refresh KeyPair err: SessionAccessKeyId: %v, SessionAccessKeySecret: %v, Expiration: %v", accessKeyId, accessKeySecret, expiration)
	}

	expirationTime, err := time.Parse("2006-01-02T15:04:05Z", expiration.(string))
	r.lastUpdateTimestamp = time.Now().Unix()
	r.credentialExpiration = int(expirationTime.Unix() - time.Now().Unix())
	r.sessionCredential = &SessionCredential{
		AccessKeyId:     accessKeyId.(string),
		AccessKeySecret: accessKeySecret.(string),
	}

	return
}