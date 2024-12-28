package service

import (
	"encoding/json"
	"log"
)

// LogRequest logs the method name and parameters for any service request
func LogRequest(method string, params interface{}) {
	reqJSON, _ := json.Marshal(params)
	log.Printf("[REQUEST] %s - %s", method, string(reqJSON))
}
