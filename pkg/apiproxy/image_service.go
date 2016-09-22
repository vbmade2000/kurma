// Copyright 2015-2016 Apcera Inc. All rights reserved.

package apiproxy

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/apcera/kurma/pkg/apiclient"
)

type ImageService struct {
	server *Server
}

func (s *Server) imageCreateRequest(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()
	image, err := s.client.CreateImage(req.Body)
	if err != nil {
		s.log.Errorf("Failed create image: %v", err)
		http.Error(w, "Failed to create image", 500)
		return
	}

	w.WriteHeader(http.StatusCreated)
	resp := &apiclient.ImageResponse{Image: image}
	json.NewEncoder(w).Encode(resp)
}

// imageFetchRequest is a handler for requests instructing the daemon to fetch
// and create a particular image.
func (s *Server) imageFetchRequest(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	var imageFetchRequest *apiclient.ImageFetchRequest
	if err := json.Unmarshal(req.Body, &imageFetchRequest); err != nil {
		s.log.Errorf("Failed to unmarshal request body: %s", err)
		http.Error(w, "Failed to parse request body", http.StatusBadRequest)
		return
	}

	image, err := s.client.FetchImage(imageFetchRequest.ImageURI, imageFetchRequest.FetchConfig)
	if err != nil {
		s.log.Errorf("Failed to fetch image %q: %s", imageFetchRequest.ImageURI, err)
		http.Error(w, "Failed to fetch image", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	resp := &apiclient.ImageResponse{Image: image}
	json.NewEncoder(w).Encode(resp)
}

func (s *ImageService) List(r *http.Request, args *apiclient.None, resp *apiclient.ImageListResponse) error {
	images, err := s.server.client.ListImages()
	if err != nil {
		return err
	}
	resp.Images = images
	return nil
}

func (s *ImageService) Get(r *http.Request, hash *string, resp *apiclient.ImageResponse) error {
	if hash == nil {
		return fmt.Errorf("no image hash was specified")
	}
	image, err := s.server.client.GetImage(*hash)
	if err != nil {
		return err
	}
	resp.Image = image
	return nil
}

func (s *ImageService) Delete(r *http.Request, hash *string, resp *apiclient.ImageResponse) error {
	if hash == nil {
		return fmt.Errorf("no image hash was specified")
	}
	return s.server.client.DeleteImage(*hash)
}
