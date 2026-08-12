package vshell

// Host is a managed client / host as returned by the client list API.
// Field names mirror the JSON returned by v_shell (capitalised keys).
type Host struct {
	Id            int    `json:"Id"`
	IsConnect     bool   `json:"IsConnect"`
	VerifyKey     string `json:"VerifyKey"`
	Tp            string `json:"Tp"`
	Addr          string `json:"Addr"`
	Remark        string `json:"Remark"`
	Status        bool   `json:"Status"`
	LocalIP       string `json:"LocalIP"`
	UserName      string `json:"UserName"`
	HostName      string `json:"HostName"`
	Location      string `json:"Location"`
	OsName        string `json:"OsName"`
	ProcessName   string `json:"ProcessName"`
	PingCheckTime int64  `json:"PingCheckTime"`
	RateLimit     int64  `json:"RateLimit"`
	NoStore       bool   `json:"NoStore"`
	NoDisplay     bool   `json:"NoDisplay"`
	MaxConn       int    `json:"MaxConn"`
	NowConn       int    `json:"NowConn"`
}

// HostListResult is the payload of POST /client/list.
type HostListResult struct {
	ClientCount      int    `json:"clientCount"`
	ClientOnlineCount int   `json:"clientOnlineCount"`
	Items            []Host `json:"items"`
	Total            int    `json:"total"`
}

// FileEntry is one entry in a directory listing.
type FileEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Time  string `json:"time"`
	Mode  string `json:"mode"`
	Size  int64  `json:"size"`
}

// FileListResult is the payload of POST /file/ls.
type FileListResult struct {
	Items []FileEntry `json:"items"`
	Total int         `json:"total"`
}

// DiskEntry is one mount / disk from POST /file/getdisk.
type DiskEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Time  string `json:"time"`
	Mode  string `json:"mode"`
	Size  int64  `json:"size"`
}

// CatResult is the payload of POST /file/cat.
type CatResult struct {
	Content string `json:"content"`
}
