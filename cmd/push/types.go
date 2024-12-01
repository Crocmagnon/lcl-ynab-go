package main

// types available at https://api.ynab.com/v1#/Transactions/createTransaction

type TransactionsPayload struct {
	Transactions []Transaction `json:"transactions"`
}

type Transaction struct {
	AccountID string `json:"account_id,omitempty"`
	Date      string `json:"date,omitempty"`
	Amount    int    `json:"amount,omitempty"`
	PayeeName string `json:"payee_name,omitempty"`
	Memo      string `json:"memo,omitempty"`
	Cleared   string `json:"cleared,omitempty"`
	ImportID  string `json:"import_id,omitempty"`
}

type TransactionsResponse struct {
	Data struct {
		DuplicateImportIDs []string `json:"duplicate_import_ids"`
	} `json:"data"`
}
