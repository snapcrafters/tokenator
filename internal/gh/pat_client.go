package gh

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/snapcrafters/tokenator/internal/config"
	"golang.org/x/net/publicsuffix"
	"golang.org/x/sync/errgroup"
)

// PAT represents a Github Personal Access Token.
type PAT struct {
	ID          string
	Name        string
	Token       string
	deleteToken string
}

// Delete removes the PAT from the Github account.
func (p *PAT) Delete(pc *PATClient) error {
	if ok, err := pc.login(); !ok {
		return fmt.Errorf("error deleting personal access token: %w", err)
	}

	fields := url.Values{}
	fields.Set("_method", "delete")
	fields.Set("authenticity_token", p.deleteToken)

	url := fmt.Sprintf("https://github.com/settings/personal-access-tokens/%s", p.ID)

	_, err := pc.postForm(url, fields)
	if err != nil {
		return fmt.Errorf("error deleting personal access token: %w", err)
	}

	slog.Debug("deleted personal access token", "token_name", p.Name, "token_id", p.ID)
	return nil
}

// PATClient represents an http.Client that can be logged into Github, retaining any
// session cookies, and used for listing, creating, and deleting PATs.
type PATClient struct {
	username string
	password string
	c        *http.Client
}

// NewPATClient constructs a new PATClient and returns it.
func NewPATClient(credentials config.LoginCredentials) *PATClient {
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})

	return &PATClient{
		username: credentials.Login,
		password: credentials.Password,
		c:        &http.Client{Jar: jar},
	}
}

// List returns a list of PATs associated with a Github account.
// The filter arg will ensure that only tokens containing the filter param
// in their name feature in the list.
func (pc *PATClient) List(filter string) ([]*PAT, error) {
	if ok, err := pc.login(); !ok {
		return nil, fmt.Errorf("failed to login to Github: %w", err)
	}

	// Grab the first page of access tokens
	doc, err := pc.getWebpage("https://github.com/settings/tokens?page=1&type=beta")
	if err != nil {
		return nil, fmt.Errorf("failed to get personal access tokens page: %w", err)
	}

	// Add the access tokens from this page to the list
	accessTokens := []*PAT{}
	accessTokens = append(accessTokens, pc.parsePATListPage(doc, filter)...)

	// Get the total number of pages of access tokens
	pageCount, _ := strconv.Atoi(doc.Find(".pagination > .current").AttrOr("data-total-pages", "1"))

	// Create a wait group so we can easily process the remaining pages concurrently
	errs := errgroup.Group{}

	// Iterate through the pages, collecting the access tokens
	for i := 2; i < pageCount+1; i++ {
		j := i
		errs.Go(func() error {
			url := fmt.Sprintf("https://github.com/settings/tokens?page=%d&type=beta", j)
			doc, err := pc.getWebpage(url)
			if err != nil {
				return fmt.Errorf("failed to parse personal access tokens page %d", j)
			}

			accessTokens = append(accessTokens, pc.parsePATListPage(doc, filter)...)
			return nil
		})
	}

	err = errs.Wait()
	if err != nil {
		return nil, err
	}

	return accessTokens, nil
}

// Create adds a new PAT to the logged in account scoped to the specified repos.
// At present the scope cannot be modified, and gives metadata read access, and
// contents read/write access. Token expiry defaults to now + 1 year.
func (pc *PATClient) Create(name string, repos []string, resourceOwner string) (*PAT, error) {
	if ok, err := pc.login(); !ok {
		return nil, fmt.Errorf("%w", err)
	}

	doc, err := pc.getWebpage("https://github.com/settings/personal-access-tokens/new")
	if err != nil {
		return nil, fmt.Errorf("failed to get the personal access token form: %w", err)
	}

	createToken, ok := doc.Find("#new_user_programmatic_access input[name=authenticity_token]").Attr("value")
	if !ok {
		return nil, fmt.Errorf("failed to identify authenticity token on personal access token form")
	}

	repoIDs := []string{}
	for _, repo := range repos {
		r := strings.Split(repo, "/")

		id, err := pc.getRepositoryID(r[0], r[1])
		if err != nil {
			return nil, fmt.Errorf("failed to get repo ID for %s: %w", repo, err)
		}

		repoIDs = append(repoIDs, id)
	}

	expiry := time.Now().AddDate(1, 0, 0).Format("2006-01-02")

	fields := url.Values{}
	fields.Set("authenticity_token", createToken)
	fields.Set("user_programmatic_access[name]", name)
	fields.Set("user_programmatic_access[default_expires_at]", "custom")
	fields.Set("user_programmatic_access[custom_expires_at]", expiry)
	fields.Set("user_programmatic_access[description]", "")
	fields.Set("target_name", resourceOwner)
	fields.Set("install_target", "selected")
	fields.Set("integration[default_permissions][contents]", "write")
	fields.Set("integration[default_permissions][metadata]", "read")

	for _, id := range repoIDs {
		fields.Add("repository_ids[]", id)
	}

	doc, err = pc.postForm("https://github.com/settings/personal-access-tokens", fields)
	if err != nil {
		return nil, fmt.Errorf("failed to POST personal access token form: %w", err)
	}

	tokenElem := doc.Find(".access-token").First()

	tokenValue, ok := tokenElem.Find("#new-access-token").Attr("value")
	if !ok {
		errorMsg := doc.Find(".error,.flash-error.flash-full").Text()
		return nil, fmt.Errorf("%s", strings.ToLower(errorMsg))
	}

	tokenId, ok := tokenElem.Attr("data-id")
	if !ok {
		return nil, fmt.Errorf("failed to retrieve ID of new personal access token")
	}

	deleteToken, ok := tokenElem.Find("input[name=authenticity_token]").First().Attr("value")
	if !ok {
		return nil, fmt.Errorf("failed to retrieve delete token for new personal access token")
	}

	token := &PAT{
		Name:        name,
		ID:          tokenId,
		Token:       tokenValue,
		deleteToken: deleteToken,
	}

	slog.Debug("created personal access token", "token_name", token.Name, "token_id", token.ID)
	return token, nil
}

// login is a helper method that returns early if the http client already holds a
// valid logged in session, or otherwise walks through the Github login flow.
func (pc *PATClient) login() (bool, error) {
	// First check if we're logged in
	resp, err := pc.c.Head("https://github.com/settings/")
	if err != nil {
		return false, fmt.Errorf("failed to detect Github session login status")
	}

	// If we are already logged in, the just return
	if resp.Request.URL.Path != "/login" {
		return true, nil
	}

	doc, err := pc.getWebpage("https://github.com/login")
	if err != nil {
		return false, fmt.Errorf("failed to parse Github login page")
	}

	// Find the hidden form fields
	fields := url.Values{}

	// Populate the form submission with the hidden fields served by the login page
	doc.Find("form input[type='hidden']").Each(func(i int, s *goquery.Selection) {
		name, _ := s.Attr("name")
		value, _ := s.Attr("value")
		fields.Set(name, value)
	})

	fields.Set("login", pc.username)
	fields.Set("password", pc.password)

	doc, err = pc.postForm("https://github.com/session", fields)
	if err != nil {
		return false, fmt.Errorf("failed to parse Github login form response")
	}

	if len(doc.Find(".flash-full.flash-error").Nodes) > 0 {
		errorMsg := doc.Find(".flash-full.flash-error").First().Text()
		return false, fmt.Errorf(strings.ToLower(errorMsg))
	}

	return true, nil
}

// parsePATListPage returns a list of PATs, constructed from those listed on the Github UI
func (pc *PATClient) parsePATListPage(doc *goquery.Document, filter string) []*PAT {
	accessTokens := []*PAT{}
	doc.Find(".access-token").Each(func(i int, s *goquery.Selection) {
		name := s.Find("a").Text()

		// Don't include items that don't match the filter.
		if !strings.Contains(name, filter) {
			return
		}

		accessTokens = append(accessTokens, &PAT{
			ID:          s.AttrOr("data-id", ""),
			Name:        name,
			deleteToken: s.Find("input[name=authenticity_token]").AttrOr("value", ""),
		})
	})

	return accessTokens
}

// getRepositoryID is a helper method that fetches the underlying ID of the repository based
// on the owner/repo name. For example "snapcrafters/ci" -> 223043.
func (pc *PATClient) getRepositoryID(owner string, repo string) (string, error) {
	req, err := http.NewRequest("GET", "https://github.com/settings/personal-access-tokens/suggestions", nil)
	if err != nil {
		return "", fmt.Errorf("failed to setup request to repository suggestions endpoint")
	}

	q := req.URL.Query()
	q.Add("target_name", owner)
	q.Add("q", repo)
	req.URL.RawQuery = q.Encode()

	req.Header.Add("Accept", "text/fragment+html")

	resp, err := pc.c.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to poll repository suggestions endpoint")
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to parse repository suggestions endpoint")
	}

	// Get the ID from the remove button that's rendered in the suggestions
	id, ok := doc.Find(fmt.Sprintf("[aria-label='Remove %s']", repo)).Attr("value")
	if !ok {
		return "", fmt.Errorf("failed to find repository id for %s/%s", owner, repo)
	}

	return id, err
}

func (pc *PATClient) getWebpage(url string) (*goquery.Document, error) {
	resp, err := pc.c.Get(url)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

func (pc *PATClient) postForm(url string, fields url.Values) (*goquery.Document, error) {
	resp, err := pc.c.PostForm(url, fields)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	return doc, nil
}
