package graphhelper

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"encoding/csv"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	auth "github.com/microsoft/kiota-authentication-azure-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	msgraphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

type GraphHelper struct {
	clientSecretCredential *azidentity.ClientSecretCredential
	appClient              *msgraphsdk.GraphServiceClient
}

func NewGraphHelper() *GraphHelper {
	g := &GraphHelper{}
	return g
}

func (g *GraphHelper) InitializeGraphForAppAuth() error {
	clientId := os.Getenv("CLIENT_ID")
	tenantId := os.Getenv("TENANT_ID")
	clientSecret := os.Getenv("CLIENT_SECRET")
	credential, err := azidentity.NewClientSecretCredential(tenantId, clientId, clientSecret, nil)
	if err != nil {
		return err
	}

	g.clientSecretCredential = credential

	// Create an auth provider using the credential
	authProvider, err := auth.NewAzureIdentityAuthenticationProviderWithScopes(g.clientSecretCredential, []string{
		"https://graph.microsoft.com/.default",
	})
	if err != nil {
		return err
	}

	// Create a request adapter using the auth provider
	adapter, err := msgraphsdk.NewGraphRequestAdapter(authProvider)
	if err != nil {
		return err
	}

	// Create a Graph client using request adapter
	client := msgraphsdk.NewGraphServiceClient(adapter)
	g.appClient = client

	return nil
}

func (g *GraphHelper) GetAppToken() (*string, error) {
	token, err := g.clientSecretCredential.GetToken(context.Background(), policy.TokenRequestOptions{
		Scopes: []string{
			"https://graph.microsoft.com/.default",
		},
	})
	if err != nil {
		return nil, err
	}

	return &token.Token, nil
}

func (g *GraphHelper) GetUsers() {
	var data [][]string

	// Create File
	file, err := os.Create("stale_users_SAML.csv")
	if err != nil {
		log.Fatalln("failed to open file", err)
	}
	defer file.Close()
	w := csv.NewWriter(file)
	defer w.Flush()

	// Create user query
	query := users.UsersRequestBuilderGetQueryParameters{
		// Only request specific properties
		Select: []string{"displayName", "id", "mail", "signInActivity"},
		// Sort by display name
		Orderby: []string{"displayName"},
	}

	// Get users
	users, err := g.appClient.Users().
		Get(context.Background(),
			&users.UsersRequestBuilderGetRequestConfiguration{
				QueryParameters: &query,
			})
	if err != nil {
		log.Panicf("Error getting users: %v", err)
	}

	// Create a new page iterator
	pageIterator, err := msgraphcore.NewPageIterator[*models.User](
		users,
		g.appClient.GetAdapter(),
		models.CreateUserCollectionResponseFromDiscriminatorValue)
	if err != nil {
		log.Fatalf("Error creating page iterator: %v\n", err)
	}

	// Iterate over all pages
	err = pageIterator.Iterate(
		context.Background(),
		func(user *models.User) bool {
			if user.GetMail() != nil {
				res := strings.Split(*user.GetMail(), "@")
				if res[len(res)-1] == "acu.edu" {
					noEmail := "NO EMAIL"
					email := user.GetMail()
					if email == nil {
						email = &noEmail
					}
					if user.GetSignInActivity() != nil {
						noSignIn := "NO SIGN IN OR LAST SIGN IN BEFORE April 2020"
						signIn := user.GetSignInActivity().GetLastSignInDateTime()
						if signIn == nil {
							data = append(data, []string{*user.GetDisplayName(), *email, noSignIn})
						} else {
							if signIn.Before(time.Now().AddDate(0, 0, -180)) {
								data = append(data, []string{*user.GetDisplayName(), *email, signIn.String()})
							}
						}
					}
				}
			}
			// Return true to continue the iteration
			return true
		})
	w.WriteAll(data)
	if err != nil {
		log.Fatalf("Error with pages")
	}
}
