package update

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/mod/semver"
)

// Microsoft antivirus will flag the binary for "Program:Win32/Wacapew.C!ml" if we use the full github api url directly in the http get.
const (
	p1 = "https://api"
	p2 = ".github.com"
	p3 = "/repos/roffe/txlogger"
	p4 = "/releases/latest"
)

type Release struct {
	URL             string    `json:"url"`
	AssetsURL       string    `json:"assets_url"`
	UploadURL       string    `json:"upload_url"`
	HTMLURL         string    `json:"html_url"`
	ID              int       `json:"id"`
	Author          Author    `json:"author"`
	NodeID          string    `json:"node_id"`
	TagName         string    `json:"tag_name"`
	TargetCommitish string    `json:"target_commitish"`
	Name            string    `json:"name"`
	Draft           bool      `json:"draft"`
	Prerelease      bool      `json:"prerelease"`
	CreatedAt       time.Time `json:"created_at"`
	PublishedAt     time.Time `json:"published_at"`
	Assets          []Assets  `json:"assets"`
	TarballURL      string    `json:"tarball_url"`
	ZipballURL      string    `json:"zipball_url"`
	Body            string    `json:"body"`
}
type Author struct {
	Login             string `json:"login"`
	ID                int    `json:"id"`
	NodeID            string `json:"node_id"`
	AvatarURL         string `json:"avatar_url"`
	GravatarID        string `json:"gravatar_id"`
	URL               string `json:"url"`
	HTMLURL           string `json:"html_url"`
	FollowersURL      string `json:"followers_url"`
	FollowingURL      string `json:"following_url"`
	GistsURL          string `json:"gists_url"`
	StarredURL        string `json:"starred_url"`
	SubscriptionsURL  string `json:"subscriptions_url"`
	OrganizationsURL  string `json:"organizations_url"`
	ReposURL          string `json:"repos_url"`
	EventsURL         string `json:"events_url"`
	ReceivedEventsURL string `json:"received_events_url"`
	Type              string `json:"type"`
	SiteAdmin         bool   `json:"site_admin"`
}
type Uploader struct {
	Login             string `json:"login"`
	ID                int    `json:"id"`
	NodeID            string `json:"node_id"`
	AvatarURL         string `json:"avatar_url"`
	GravatarID        string `json:"gravatar_id"`
	URL               string `json:"url"`
	HTMLURL           string `json:"html_url"`
	FollowersURL      string `json:"followers_url"`
	FollowingURL      string `json:"following_url"`
	GistsURL          string `json:"gists_url"`
	StarredURL        string `json:"starred_url"`
	SubscriptionsURL  string `json:"subscriptions_url"`
	OrganizationsURL  string `json:"organizations_url"`
	ReposURL          string `json:"repos_url"`
	EventsURL         string `json:"events_url"`
	ReceivedEventsURL string `json:"received_events_url"`
	Type              string `json:"type"`
	SiteAdmin         bool   `json:"site_admin"`
}
type Assets struct {
	URL                string      `json:"url"`
	ID                 int         `json:"id"`
	NodeID             string      `json:"node_id"`
	Name               string      `json:"name"`
	Label              interface{} `json:"label"`
	Uploader           Uploader    `json:"uploader"`
	ContentType        string      `json:"content_type"`
	State              string      `json:"state"`
	Size               int         `json:"size"`
	DownloadCount      int         `json:"download_count"`
	CreatedAt          time.Time   `json:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at"`
	BrowserDownloadURL string      `json:"browser_download_url"`
}

func UpdateCheck(a fyne.App, mw fyne.Window) {
	isLatest, latestVersion := IsLatest("v" + a.Metadata().Version)
	if !isLatest {
		u, err := url.Parse("https://txlogger.com")
		if err != nil {
			panic(err)
		}
		link := widget.NewHyperlink("Get it at txlogger.com", u)
		link.Alignment = fyne.TextAlignLeading
		link.TextStyle = fyne.TextStyle{Bold: true}
		dialog.ShowCustom(
			"Update available",
			"Close",
			container.NewVBox(
				widget.NewLabel("Current version: v"+a.Metadata().Version),
				widget.NewLabel("Latest version: "+latestVersion),
				link,
			),
			mw,
		)
	} else {
		dialog.ShowInformation("No update available", "You are running the latest version", mw)
	}
}

func GetLatest() (*Release, error) {
	latest := new(Release)
	b, err := httpGetBody(p1 + p2 + p3 + p4)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &latest); err != nil {
		return nil, err
	}
	return latest, nil
}

/*
	func GetReleases() []*Release {
		var releases []*Release
		b, err := httpGetBody("https://api.github.com/repos/roffe/txlogger/releases")
		if err != nil {
			return nil
		}
		if err := json.Unmarshal(b, &releases); err != nil {
			return nil
		}
		return releases
	}
*/

func httpGetBody(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}

func IsLatest(version string) (bool, string) {
	latest, err := GetLatest()
	if err != nil {
		return true, version
	}
	//	log.Println("latest.TagName:", latest.TagName)
	//	log.Println("current.Version:", version)
	return !(semver.Compare(latest.TagName, version) > 0), latest.TagName
}
