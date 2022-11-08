// Package sender implements the upload sender.
package sender

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/closer"
	"github.com/m-lab/ndt-server/ndt7/measurer"
	ndt7metrics "github.com/m-lab/ndt-server/ndt7/metrics"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/ping"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/marten-seemann/webtransport-go"
)

func writeJSON(str webtransport.SendStream, v interface{}) error {
	json.NewEncoder(os.Stdout).Encode(v)
	err1 := json.NewEncoder(str).Encode(v)
	err2 := str.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

// Start sends measurement messages (status messages) to the client conn. Each
// measurement message will also be saved to data.
//
// Liveness guarantee: the sender will not be stuck sending for more than the
// MaxRuntime of the subtest. This is enforced by setting the write deadline to
// Time.Now() + MaxRuntime.
func Start(ctx context.Context, conn *websocket.Conn, data *model.ArchivalData) error {
	logging.Logger.Debug("sender: start")
	proto := ndt7metrics.ConnLabel(conn)

	// Start collecting connection measurements. Measurements will be sent to
	// src until DefaultRuntime, when the src channel is closed.
	mr := measurer.New(conn, data.UUID)
	src := mr.Start(ctx, spec.DefaultRuntime)
	defer logging.Logger.Debug("sender: stop")
	defer mr.Stop(src)

	deadline := time.Now().Add(spec.MaxRuntime)
	err := conn.SetWriteDeadline(deadline) // Liveness!
	if err != nil {
		logging.Logger.WithError(err).Warn("sender: conn.SetWriteDeadline failed")
		ndt7metrics.ClientSenderErrors.WithLabelValues(
			proto, string(spec.SubtestUpload), "set-write-deadline").Inc()
		return err
	}

	// Record measurement start time, and prepare recording of the endtime on return.
	data.StartTime = time.Now().UTC()
	defer func() {
		data.EndTime = time.Now().UTC()
	}()
	for {
		m, ok := <-src
		if !ok { // This means that the previous step has terminated
			closer.StartClosing(conn)
			ndt7metrics.ClientSenderErrors.WithLabelValues(
				proto, string(spec.SubtestUpload), "measurer-closed").Inc()
			return nil
		}
		if err := conn.WriteJSON(m); err != nil {
			logging.Logger.WithError(err).Warn("sender: conn.WriteJSON failed")
			ndt7metrics.ClientSenderErrors.WithLabelValues(
				proto, string(spec.SubtestUpload), "write-json").Inc()
			return err
		}
		// Only save measurements sent to the client.
		data.ServerMeasurements = append(data.ServerMeasurements, m)
		if err := ping.SendTicks(conn, deadline); err != nil {
			logging.Logger.WithError(err).Warn("sender: ping.SendTicks failed")
			ndt7metrics.ClientSenderErrors.WithLabelValues(
				proto, string(spec.SubtestUpload), "ping-send-ticks").Inc()
			return err
		}
	}
}



// Start sends measurement messages (status messages) to the client conn. Each
// measurement message will also be saved to data.
//
// Liveness guarantee: the sender will not be stuck sending for more than the
// MaxRuntime of the subtest. This is enforced by setting the write deadline to
// Time.Now() + MaxRuntime.
func StartWebTransport(ctx context.Context, sess *webtransport.Session, data *model.ArchivalData, mr *measurer.WebTransportMeasurer) error {
	logging.Logger.Debug("sender: start")
	proto := "ndt+webtransport"

	// Start collecting connection measurements. Measurements will be sent to
	// src until DefaultRuntime, when the src channel is closed.
	src := mr.Start(ctx, spec.DefaultRuntime)
	defer logging.Logger.Debug("sender: stop")
	defer mr.Stop(src)

	deadline := time.Now().Add(spec.MaxRuntime)
	str, err := sess.OpenUniStream()
	if err != nil {
		logging.Logger.WithError(err).Warn("sender: sess.OpenUniStream failed")
		ndt7metrics.ClientSenderErrors.WithLabelValues(
			proto, string(spec.SubtestUpload), "open-uni-stream").Inc()
		return err
	}
	err = str.SetWriteDeadline(deadline) // Liveness!
	if err != nil {
		logging.Logger.WithError(err).Warn("sender: str.SetWriteDeadline failed")
		ndt7metrics.ClientSenderErrors.WithLabelValues(
			proto, string(spec.SubtestUpload), "str-set-write-deadline").Inc()
		return err
	}

	// Record measurement start time, and prepare recording of the endtime on return.
	data.StartTime = time.Now().UTC()
	defer func() {
		data.EndTime = time.Now().UTC()
	}()
	for {
		m, ok := <-src
		if !ok { // This means that the previous step has terminated
			// TODO: start closing
			// closer.StartClosing(str)
			ndt7metrics.ClientSenderErrors.WithLabelValues(
				proto, string(spec.SubtestUpload), "measurer-closed").Inc()
			return nil
		}
		
		m.QUICInfo = &model.QUICInfo{
			QUICStreamBytesReceived: int64(mr.BytesReceived),
		}

		jsonStr, err := sess.OpenUniStreamSync(ctx)
		if err != nil {
			return err
		}
		
		if err := writeJSON(jsonStr, m); err != nil {
			fmt.Println("ERROR", err)
			logging.Logger.WithError(err).Warn("sender: conn.WriteJSON failed")
			ndt7metrics.ClientSenderErrors.WithLabelValues(
				proto, string(spec.SubtestUpload), "write-json").Inc()
			return err
		}
		jsonStr.Close()
		// Only save measurements sent to the client.
		data.ServerMeasurements = append(data.ServerMeasurements, m)
		if err := ping.SendTicksWebTransport(sess, deadline); err != nil {
			logging.Logger.WithError(err).Warn("sender: ping.SendTicks failed")
			ndt7metrics.ClientSenderErrors.WithLabelValues(
				proto, string(spec.SubtestUpload), "ping-send-ticks").Inc()
			return err
		}
	}
}
