/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"context"
	"fmt"
	"strings"

	api "github.com/yandex-cloud/go-genproto/yandex/cloud/lockbox/v1"
	"github.com/yandex-cloud/go-sdk/iamkey"
	"google.golang.org/grpc"

	"github.com/external-secrets/external-secrets/pkg/provider/yandex/common"
)

// Real/gRPC implementation of LockboxClient.
type grpcLockboxClient struct {
	lockboxPayloadClient api.PayloadServiceClient
	lockboxSecretClient  api.SecretServiceClient
}

func NewGrpcLockboxClient(ctx context.Context, apiEndpoint string, authorizedKey *iamkey.Key, caCertificate []byte) (LockboxClient, error) {
	conn, err := common.NewGrpcConnection(
		// Required lockbox.payloadViewer role
		ctx,
		apiEndpoint,
		"lockbox-payload", // taken from https://api.cloud.yandex.net/endpoints
		authorizedKey,
		caCertificate,
	)
	if err != nil {
		return nil, err
	}
	conn2, err := common.NewGrpcConnection(
		// Required lockbox.viewer role
		ctx,
		apiEndpoint,
		"lockbox", // taken from https://api.cloud.yandex.net/endpoints
		authorizedKey,
		caCertificate,
	)
	if err != nil {
		return nil, err
	}
	return &grpcLockboxClient{
		api.NewPayloadServiceClient(conn),
		api.NewSecretServiceClient(conn2),
	}, nil
}

func (c *grpcLockboxClient) GetSecretIDByName(ctx context.Context, iamToken, folderID, secretName string) (string, error) {
	list, err := c.lockboxSecretClient.List(
		ctx,
		&api.ListSecretsRequest{
			FolderId: folderID,
			PageSize: 1000,
		},
		grpc.PerRPCCredentials(common.PerRPCCredentials{IamToken: iamToken}),
	)
	if err != nil {
		return "", err
	}
	for _, secret := range list.Secrets {
		if strings.TrimSpace(secret.Name) == strings.TrimSpace(secretName) {
			return secret.Id, nil
		}
	}
	return "", fmt.Errorf("secret name %s not found in folder %s", secretName, folderID)
}

func (c *grpcLockboxClient) GetPayloadEntries(ctx context.Context, iamToken, folderID, secretIDOrName, versionID string) ([]*api.Payload_Entry, error) {
	secretID := secretIDOrName
	var payload *api.Payload

	// If the folderID is provided in the SecretStore, we can attempt to retrieve the secret by its name
	if folderID != "" {
		var err error
		secretID, err = c.GetSecretIDByName(ctx, iamToken, folderID, secretIDOrName)

		// Try to get the secret by name
		response, err := c.lockboxPayloadClient.GetEx(
			ctx,
			&api.GetExRequest{
				Identifier: &api.GetExRequest_FolderAndName{
					FolderAndName: &api.FolderAndName{
						FolderId:   folderID,
						SecretName: secretIDOrName,
					},
				},
				VersionId: versionID,
			},
			grpc.PerRPCCredentials(common.PerRPCCredentials{IamToken: iamToken}),
		)
		if err == nil {
			// Convert the response of GetEx method to api.Payload
			payload = &api.Payload{}
			payload.VersionId = response.VersionId
			payload.Entries = []*api.Payload_Entry{}
			for key, value := range response.Entries {
				payload.Entries = append(payload.Entries, &api.Payload_Entry{
					Key:   key,
					Value: &api.Payload_Entry_TextValue{TextValue: string(value)},
				})
			}
			return payload.Entries, nil
		}

		if err != nil {
			if len(secretIDOrName) == 20 && strings.HasPrefix(secretIDOrName, "e6q") {
				// Try to get the secret by ID
				payload, err = c.lockboxPayloadClient.Get(
					ctx,
					&api.GetPayloadRequest{
						SecretId:  secretID,
						VersionId: versionID,
					},
					grpc.PerRPCCredentials(common.PerRPCCredentials{IamToken: iamToken}),
				)
				if err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
		}
	}

	return payload.Entries, nil
}
