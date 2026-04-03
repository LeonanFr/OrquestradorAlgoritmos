package executorclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"orchestrator/internal/models"
)

type Node struct {
	URL     string
	Healthy bool
}

type FetcherFunc func() ([]string, error)

type Client struct {
	nodes       []*Node
	mu          sync.Mutex
	roundRobin  int
	timeout     time.Duration
	client      *http.Client
	hcClient    *http.Client
	fetcher     FetcherFunc
	secretToken string
}

func NewClient(fetcher FetcherFunc, token string, timeoutSec, healthIntervalSec int) *Client {
	c := &Client{
		nodes:       make([]*Node, 0),
		timeout:     time.Duration(timeoutSec) * time.Second,
		client:      &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
		hcClient:    &http.Client{Timeout: 5 * time.Second},
		fetcher:     fetcher,
		secretToken: token,
	}

	c.syncAndCheckNodes()
	go c.healthCheckLoop(time.Duration(healthIntervalSec) * time.Second)
	return c
}

func (c *Client) healthCheckLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		c.syncAndCheckNodes()
	}
}

func (c *Client) syncAndCheckNodes() {
	urls, err := c.fetcher()
	if err != nil {
		log.Printf("erro ao buscar executores no banco: %v", err)
		return
	}

	newNodes := make([]*Node, len(urls))
	for i, u := range urls {
		healthy := false
		resp, err := c.hcClient.Get(u + "/health")
		if err == nil && resp != nil {
			if resp.StatusCode == http.StatusOK {
				healthy = true
			}
			_ = resp.Body.Close()
		} else {
			log.Printf("executor offline ou erro no health check: %s - %v", u, err)
		}
		newNodes[i] = &Node{URL: u, Healthy: healthy}
	}

	c.mu.Lock()
	c.nodes = newNodes
	c.mu.Unlock()
}

func (c *Client) getNextNode() *Node {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.nodes) == 0 {
		return nil
	}

	startIdx := c.roundRobin
	for i := 0; i < len(c.nodes); i++ {
		idx := (startIdx + i) % len(c.nodes)
		if c.nodes[idx].Healthy {
			c.roundRobin = (idx + 1) % len(c.nodes)
			return c.nodes[idx]
		}
	}
	return nil
}

func (c *Client) Execute(ctx context.Context, payload models.ExecutorPayload) (*models.ExecutorResponse, string, error) {
	node := c.getNextNode()
	if node == nil {
		return nil, "", fmt.Errorf("nenhum executor disponivel ou saudavel")
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, node.URL, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, node.URL+"/execute", bytes.NewReader(data))
	if err != nil {
		return nil, node.URL, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secretToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.secretToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, node.URL, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, node.URL, fmt.Errorf("executor retornou status %d", resp.StatusCode)
	}

	var execRes models.ExecutorResponse
	if err := json.NewDecoder(resp.Body).Decode(&execRes); err != nil {
		return nil, node.URL, err
	}

	return &execRes, node.URL, nil
}
