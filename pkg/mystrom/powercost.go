package mystrom

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func GetEnergyPrice(municipalityID string, energyCategory string) (float64, error) {
	energyPriceAPIURL := "https://www.strompreis.abn.elcom.admin.ch/api/graphql"

	payload := strings.NewReader(fmt.Sprintf(`{
  "query": "query Observations($locale: String!, $priceComponent: PriceComponent!, $filters: ObservationFilters!, $observationKind: ObservationKind) {\n  observations(\n    locale: $locale\n    filters: $filters\n    observationKind: $observationKind\n  ) {\n    ...operatorObservationFields\n    __typename\n  }\n}\n\nfragment operatorObservationFields on OperatorObservation {\n  period\n  municipality\n  municipalityLabel\n  operator\n  operatorLabel\n  canton\n  cantonLabel\n  category\n  value(priceComponent: $priceComponent)\n}",
  "operationName": "Observations",
  "variables": {
    "locale": "de",
    "priceComponent": "total",
    "filters": {
      "period": ["%s"],
      "category": ["%s"],
      "product": ["standard"],
      "municipality": ["%s"]
    }
  }
}
`, time.Now().Format("2006"), energyCategory, municipalityID))

	req, _ := http.NewRequest("POST", energyPriceAPIURL, payload)

	req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}

	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, err
	}

	jsonData := json.NewDecoder(strings.NewReader(string(body)))
	var data map[string]interface{}
	jsonData.Decode(&data)

	if len(data["data"].(map[string]interface{})["observations"].([]interface{})) == 0 {
		return 0, fmt.Errorf("energy category %s not found", energyCategory)
	}
	energyPrice := data["data"].(map[string]interface{})["observations"].([]interface{})[0].(map[string]interface{})["value"].(float64)

	return energyPrice, nil
}

func GetMunicipalityID(municipality string) (string, error) {
	municipalityAPIURL := fmt.Sprintf("https://www.agvchapp.bfs.admin.ch/api/communes/snapshot?date=%s", time.Now().Format("02-01-2006"))
	resp, err := http.Get(municipalityAPIURL)
	if err != nil {
		return "", err
	}

	r := csv.NewReader(resp.Body)
	records, err := r.ReadAll()
	if err != nil {
		return "", err
	}
	for _, record := range records {
		if record[6] == municipality {
			return record[1], nil
		}
	}
	return "", fmt.Errorf("municipality %s not found", municipality)
}
