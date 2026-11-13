package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

var errSecondFactor = errors.New("UIS 动态口令验证未完成")

type authResult struct {
	Code          any      `json:"code"`
	Message       string   `json:"message"`
	LoginToken    string   `json:"loginToken"`
	Second        bool     `json:"second"`
	ModuleCodes   []string `json:"moduleCodes"`
	RequestNumber string   `json:"requestNumber"`
	AuthChainCode string   `json:"authChainCode"`
	UserName      string   `json:"userName"`
}

func totpKey(secret string) ([]byte, error) {
	compact := strings.ToUpper(strings.Join(strings.Fields(secret), ""))
	if compact == "" {
		return nil, fmt.Errorf("%w：请在「来源」中填写 UIS 绑定动态口令时提供的 Base32 密钥", errSecondFactor)
	}
	encoding := base32.StdEncoding.WithPadding(base32.NoPadding)
	if strings.Contains(compact, "=") {
		encoding = base32.StdEncoding
	}
	key, err := encoding.DecodeString(compact)
	if err != nil || len(key) == 0 || encoding.EncodeToString(key) != compact {
		return nil, fmt.Errorf("%w：请检查绑定时的 Base32 密钥", errSecondFactor)
	}
	return key, nil
}

// UIS uses RFC 6238 with SHA-1, six digits and a 30-second period.
func totpCode(key []byte, at time.Time) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(at.Unix()/30))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	value := binary.BigEndian.Uint32(digest[offset:]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000)
}

func (u *uis) completeTOTP(ctx context.Context, challenge authResult, username, entity, requestType, lck string, at time.Time) (string, error) {
	if !slices.Contains(challenge.ModuleCodes, "userAndOtp") {
		return "", fmt.Errorf("%w：当前账号未提供动态口令方式，请检查 UIS 绑定设置", errSecondFactor)
	}
	if challenge.RequestNumber == "" || challenge.AuthChainCode == "" {
		return "", fmt.Errorf("%w：登录响应缺少验证上下文，请重新登录", errSecondFactor)
	}
	key, err := totpKey(u.totpSecret)
	if err != nil {
		return "", err
	}
	if challenge.UserName != "" {
		username = challenge.UserName
	}
	var result authResult
	err = u.postJSON(ctx, "/idp/authn/authExecute", map[string]any{
		"authModuleCode": "userAndOtp", "authChainCode": challenge.AuthChainCode, "requestNumber": challenge.RequestNumber,
		"entityId": entity, "requestType": requestType, "lck": lck,
		"authPara": map[string]string{"loginName": username, "otpCode": totpCode(key, at)},
	}, &result)
	if err != nil {
		return "", fmt.Errorf("无法读取 UIS 动态口令验证结果：%w", err)
	}
	if fmt.Sprint(result.Code) != "200" {
		return "", fmt.Errorf("%w：请检查绑定密钥和设备时间", errSecondFactor)
	}
	if result.Second || result.LoginToken == "" {
		return "", fmt.Errorf("%w：UIS 仍需要进一步验证", errSecondFactor)
	}
	return result.LoginToken, nil
}
