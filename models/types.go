package models

type InstallRequest struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	InstallMySQL   bool   `json:"install_mysql"`
	ForceOverwrite bool   `json:"force_overwrite"`
}

type InstallResponse struct {
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	RawLog     string `json:"raw_log"`
	TxAdminPIN string `json:"pin"`
	TxAdminURL string `json:"tx_url"`
	PmaUrl     string `json:"pma_url,omitempty"`
	MySQLHost  string `json:"mysql_host,omitempty"`
	MySQLDB    string `json:"mysql_db,omitempty"`
	MySQLUser  string `json:"mysql_user,omitempty"`
	MySQLPass  string `json:"mysql_pass,omitempty"`

	RootUser string `json:"root_user,omitempty"`
	RootPass string `json:"root_pass,omitempty"`
}
