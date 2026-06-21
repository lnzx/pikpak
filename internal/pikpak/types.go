package pikpak

import "time"

type QuotaMessage struct {
	Quota  Quota  `json:"quota"`
	Quotas Quotas `json:"quotas"`
}

type Quotas struct {
	CloudDownload Quota `json:"cloud_download"`
}

type Quota struct {
	Limit int64 `json:"limit,string"`
	Usage int64 `json:"usage,string"`
}

type FileStat struct {
	Kind         string    `json:"kind"`
	ID           string    `json:"id"`
	ParentID     string    `json:"parent_id"`
	Name         string    `json:"name"`
	Size         int64     `json:"size,string"`
	CreatedTime  time.Time `json:"created_time"`
	ModifiedTime time.Time `json:"modified_time"`
	Phase        string    `json:"phase"`
	Trashed      bool      `json:"trashed"`
}

type File struct {
	FileStat
	WebContentLink string `json:"web_content_link"`
	Links          struct {
		ApplicationOctetStream struct {
			URL string `json:"url"`
		} `json:"application/octet-stream"`
	} `json:"links"`
	Medias []struct {
		Link struct {
			URL string `json:"url"`
		} `json:"link"`
	} `json:"medias"`
}

type OfflineTask struct {
	ID          string `json:"id"`
	FileID      string `json:"file_id"`
	FileName    string `json:"file_name"`
	FileSize    string `json:"file_size"`
	Name        string `json:"name"`
	Phase       string `json:"phase"`
	Progress    int64  `json:"progress"`
	Message     string `json:"message"`
	CreatedTime string `json:"created_time"`
	UpdatedTime string `json:"updated_time"`
	Params      struct {
		URL string `json:"url"`
	} `json:"params"`
}

type offlineListResp struct {
	NextPageToken string        `json:"next_page_token"`
	Tasks         []OfflineTask `json:"tasks"`
}

type offlineDownloadResp struct {
	Task OfflineTask `json:"task"`
}

type filesResp struct {
	NextPageToken string     `json:"next_page_token"`
	Files         []FileStat `json:"files"`
}

type errResp struct {
	ErrorCode        int64  `json:"error_code"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (e errResp) IsError() bool {
	return e.ErrorCode != 0 || e.Error != ""
}
