// Package download implements the ndt7/server downloader.
package download

import (
	"context"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/ndt7/download/sender"
	"github.com/m-lab/ndt-server/ndt7/measurer"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/receiver"
	"github.com/marten-seemann/webtransport-go"
)

// Do implements the download subtest. The ctx argument is the parent context
// for the subtest. The conn argument is the open WebSocket connection. The data
// argument is the archival data where results are saved. All arguments are
// owned by the caller of this function.
func Do(ctx context.Context, conn *websocket.Conn, data *model.ArchivalData) error {
	// Implementation note: use child contexts so the sender is strictly time
	// bounded. After timeout, the sender closes the conn, which results in the
	// receiver completing.

	// Receive and save client-provided measurements in data.
	recv := receiver.StartDownloadReceiverAsync(ctx, conn, data)

	// Perform download and save server-measurements in data.
	// TODO: move sender.Start logic to this file.
	err := sender.Start(ctx, conn, data)

	// Block on the receiver completing to guarantee that access to data is synchronous.
	<-recv.Done()
	return err
}

// Dame as Do but for WebTransport
func DoWebTransport(ctx context.Context, sess *webtransport.Session, data *model.ArchivalData) error {
	// Implementation note: use child contexts so the sender is strictly time
	// bounded. After timeout, the sender closes the conn, which results in the
	// receiver completing.

	mr := measurer.NewWebTransport(sess, data.UUID)
	// Receive and save client-provided measurements in data.
	recv := receiver.StartWebTransportDownloadReceiverAsync(ctx, sess, data, mr)

	// Perform download and save server-measurements in data.
	// TODO: move sender.Start logic to this file.
	err := sender.StartWebTransport(ctx, sess, data, mr)

	// Block on the receiver completing to guarantee that access to data is synchronous.
	<-recv.Done()
	return err
}
