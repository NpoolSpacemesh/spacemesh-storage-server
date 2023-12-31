package api

import (
	"net"
	"net/http"

	types "github.com/NpoolSpacemesh/spacemesh-storage-server/types"

	"encoding/json"
	"fmt"

	log "github.com/EntropyPool/entropy-logger"
	httpdaemon "github.com/NpoolRD/http-daemon"
	"golang.org/x/xerrors"
)

func UploadPlot(host, port string, input types.UploadPlotInput) (*types.UploadPlotOutput, error) {
	log.Infof(log.Fields{}, "req to %v%v", "", input.PlotURL)

	addr := net.JoinHostPort(host, port)
	resp, err := httpdaemon.R().
		SetHeader("Content-Type", "application/json").
		SetBody(input).
		Post(fmt.Sprintf("http://%v%v", addr, types.UploadPlotAPI))
	if err != nil {
		log.Errorf(log.Fields{}, "heartbeat error: %v", err)
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, xerrors.Errorf("NON-200 return")
	}

	apiResp, err := httpdaemon.ParseResponse(resp)
	if err != nil {
		return nil, err
	}

	output := types.UploadPlotOutput{}
	b, _ := json.Marshal(apiResp.Body)
	err = json.Unmarshal(b, &output)

	return &output, err
}
