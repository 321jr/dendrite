// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package writers

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/mediaapi/config"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/util"
)

// uploadRequest metadata included in or derivable from an upload request
// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-media-r0-upload
// NOTE: The members come from HTTP request metadata such as headers, query parameters or can be derived from such
type uploadRequest struct {
	MediaMetadata *types.MediaMetadata
	Logger        *log.Entry
}

// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-media-r0-upload
type uploadResponse struct {
	ContentURI string `json:"content_uri"`
}

// Upload implements /upload
//
// This endpoint involves uploading potentially significant amounts of data to the homeserver.
// This implementation supports a configurable maximum file size limit in bytes. If a user tries to upload more than this, they will receive an error that their upload is too large.
// Uploaded files are processed piece-wise to avoid DoS attacks which would starve the server of memory.
// TODO: We should time out requests if they have not received any data within a configured timeout period.
func Upload(req *http.Request, cfg *config.MediaAPI) util.JSONResponse {
	r, resErr := parseAndValidateRequest(req, cfg)
	if resErr != nil {
		return *resErr
	}

	// doUpload

	return util.JSONResponse{
		Code: 200,
		JSON: uploadResponse{
			ContentURI: fmt.Sprintf("mxc://%s/%s", cfg.ServerName, r.MediaMetadata.MediaID),
		},
	}
}

// parseAndValidateRequest parses the incoming upload request to validate and extract
// all the metadata about the media being uploaded. Returns either an uploadRequest or
// an error formatted as a util.JSONResponse
func parseAndValidateRequest(req *http.Request, cfg *config.MediaAPI) (*uploadRequest, *util.JSONResponse) {
	if req.Method != "POST" {
		return nil, &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown("HTTP request method must be POST."),
		}
	}

	// authenticate user

	r := &uploadRequest{
		MediaMetadata: &types.MediaMetadata{
			Origin:             cfg.ServerName,
			ContentDisposition: types.ContentDisposition(req.Header.Get("Content-Disposition")),
			FileSizeBytes:      types.FileSizeBytes(req.ContentLength),
			ContentType:        types.ContentType(req.Header.Get("Content-Type")),
			UploadName:         types.Filename(req.FormValue("filename")),
		},
		Logger: util.GetLogger(req.Context()),
	}

	if resErr := r.Validate(cfg.MaxFileSizeBytes); resErr != nil {
		return nil, resErr
	}

	// FIXME: do we want to always override ContentDisposition here or only if
	// there is no Content-Disposition header set?
	if len(r.MediaMetadata.UploadName) > 0 {
		r.MediaMetadata.ContentDisposition = types.ContentDisposition(
			"inline; filename*=utf-8''" + url.PathEscape(string(r.MediaMetadata.UploadName)),
		)
	}

	return r, nil
}

// Validate validates the uploadRequest fields
func (r *uploadRequest) Validate(maxFileSizeBytes types.FileSizeBytes) *util.JSONResponse {
	// TODO: Any validation to be done on ContentDisposition?

	if r.MediaMetadata.FileSizeBytes < 1 {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown("HTTP Content-Length request header must be greater than zero."),
		}
	}
	if maxFileSizeBytes > 0 && r.MediaMetadata.FileSizeBytes > maxFileSizeBytes {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("HTTP Content-Length is greater than the maximum allowed upload size (%v).", maxFileSizeBytes)),
		}
	}
	// TODO: Check if the Content-Type is a valid type?
	if r.MediaMetadata.ContentType == "" {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown("HTTP Content-Type request header must be set."),
		}
	}
	// TODO: Validate filename - what are the valid characters?
	if r.MediaMetadata.UserID != "" {
		// TODO: We should put user ID parsing code into gomatrixserverlib and use that instead
		//       (see https://github.com/matrix-org/gomatrixserverlib/blob/3394e7c7003312043208aa73727d2256eea3d1f6/eventcontent.go#L347 )
		//       It should be a struct (with pointers into a single string to avoid copying) and
		//       we should update all refs to use UserID types rather than strings.
		// https://github.com/matrix-org/synapse/blob/v0.19.2/synapse/types.py#L92
		if len(r.MediaMetadata.UserID) == 0 || r.MediaMetadata.UserID[0] != '@' {
			return &util.JSONResponse{
				Code: 400,
				JSON: jsonerror.Unknown("user id must start with '@'"),
			}
		}
		parts := strings.SplitN(string(r.MediaMetadata.UserID[1:]), ":", 2)
		if len(parts) != 2 {
			return &util.JSONResponse{
				Code: 400,
				JSON: jsonerror.BadJSON("user id must be in the form @localpart:domain"),
			}
		}
	}
	return nil
}
