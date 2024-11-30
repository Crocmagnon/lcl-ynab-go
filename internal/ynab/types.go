package ynab

// types available at https://api.ynab.com/v1#/Transactions/createTransaction

type TransactionsPayload struct {
	Transactions []Transaction `json:"transactions"`
}

type Transaction struct {
	AccountId       string           `json:"account_id"`
	Date            string           `json:"date"`
	Amount          int              `json:"amount"`
	PayeeId         string           `json:"payee_id"`
	PayeeName       string           `json:"payee_name"`
	CategoryId      string           `json:"category_id"`
	Memo            string           `json:"memo"`
	Cleared         string           `json:"cleared"`
	Approved        bool             `json:"approved"`
	FlagColor       string           `json:"flag_color"`
	SubTransactions []SubTransaction `json:"subtransactions"`
	ImportId        string           `json:"import_id"`
}

type SubTransaction struct {
	Amount     int    `json:"amount"`
	PayeeId    string `json:"payee_id"`
	PayeeName  string `json:"payee_name"`
	CategoryId string `json:"category_id"`
	Memo       string `json:"memo"`
}

type TransactionsResponse struct {
	Data struct {
		Transactions []struct {
			MatchedTransactionId string `json:"matched_transaction_id"`
		} `json:"transactions"`
		DuplicateImportIds []string `json:"duplicate_import_ids"`
	} `json:"data"`
}
