package fs

import (
	"context"
	"encoding/json"
	"net/url"
	"time"

	"github.com/jstaf/onedriver/fs/graph"
	"github.com/rs/zerolog/log"
	"github.com/yousong/socketio-go/engineio"
	"github.com/yousong/socketio-go/socketio"
)

type subscriptionResponse struct {
	Context            string    `json:"@odata.context"`
	ClientState        string    `json:"clientState"`
	ExpirationDateTime time.Time `json:"expirationDateTime"`
	Id                 string    `json:"id"`
	NotificationUrl    string    `json:"notificationUrl"`
	Resource           string    `json:"resource"`
}

func (f *Filesystem) subscribeChanges() (subscriptionResponse, error) {
	subscResp := subscriptionResponse{}

	resp, err := graph.Get(f.subscribeChangesLink, f.auth)
	if err != nil {
		return subscResp, err
	}
	if err := json.Unmarshal(resp, &subscResp); err != nil {
		return subscResp, err
	}
	return subscResp, nil
}

type subscribeFunc func() (subscriptionResponse, error)
type subscription struct {
	C <-chan struct{}

	subscribe subscribeFunc
	c         chan struct{}
	closeCh   chan struct{}
	sioErrCh  chan error
}

func newSubscription(subscribe subscribeFunc) *subscription {
	s := &subscription{
		subscribe: subscribe,
		c:         make(chan struct{}),
		closeCh:   make(chan struct{}),
		sioErrCh:  make(chan error),
	}
	s.C = s.c
	return s
}

func (s *subscription) Start() {
	const (
		errRetryInterval      = 10 * time.Second
		setupEventChanTimeout = 10 * time.Second
	)
	triggerOnErrCh := make(chan struct{}, 1)
	triggerOnErr := func() {
		select {
		case triggerOnErrCh <- struct{}{}:
		default:
		}
	}
	go func() {
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()

		for {
			select {
			case <-tick.C:
			case <-s.closeCh:
				return
			}
			select {
			case <-triggerOnErrCh:
				s.trigger()
			default:
			}
		}
	}()

	for {
		resp, err := s.subscribe()
		if err != nil {
			log.Error().Err(err).Msg("make subscription")
			triggerOnErr()
			time.Sleep(errRetryInterval)
			continue
		}
		nextDur := resp.ExpirationDateTime.Sub(time.Now())
		ctx := context.Background()
		ctx, _ = context.WithTimeout(ctx, setupEventChanTimeout)
		cleanup, err := s.setupEventChan(ctx, resp.NotificationUrl)
		if err != nil {
			log.Error().Err(err).Msg("subscription chan setup")
			triggerOnErr()
			time.Sleep(errRetryInterval)
			continue
		}
		// Trigger once so subscribers can pick up deltas ocurred
		// between expiration of last subscription and start of this
		// subscription
		s.trigger()
		if bye := func() bool {
			defer cleanup()
			select {
			case <-time.After(nextDur):
			case err := <-s.sioErrCh:
				log.Warn().Err(err).Msg("socketio session error")
			case <-s.closeCh:
				return true
			}
			return false
		}(); bye {
			return
		}
	}
}

func (s *subscription) setupEventChan(ctx context.Context, urlstr string) (func(), error) {
	u, err := url.Parse(urlstr)
	if err != nil {
		return nil, err
	}
	sioc, err := socketio.DialContext(ctx, socketio.Config{
		URL:        urlstr,
		EIOVersion: engineio.EIO3,
		OnError:    s.socketioOnError,
	})
	if err != nil {
		return nil, err
	}
	ns := &socketio.Namespace{
		Name: u.RequestURI(),
		PacketHandlers: map[byte]socketio.Handler{
			socketio.PacketTypeEVENT: s.notificationHandler,
		},
	}
	if err := sioc.Connect(ctx, ns); err != nil {
		return nil, err
	}
	return func() { sioc.Close() }, err
}

func (s *subscription) notificationHandler(msg socketio.Message) {
	var evt []string
	if err := json.Unmarshal(msg.DataRaw, &evt); err != nil {
		log.Warn().Err(err).Msg("unmarshal socketio event")
		return
	}
	if len(evt) < 2 || evt[0] != "notification" {
		log.Warn().Int("len", len(evt)).Str("type", evt[0]).Msg("check event type")
		return
	}
	var n struct {
		ClientState                    string `json:"clientState"`
		SubscriptionId                 string `json:"subscriptionId"`
		SubscriptionExpirationDateTime string `json:"subscriptionExpirationDateTime"`
		UserId                         string `json:"userId"`
		Resource                       string `json:"resource"`
	}
	if err := json.Unmarshal([]byte(evt[1]), &n); err != nil {
		log.Warn().Err(err).Msg("unmarshal notification content")
		return
	}
	log.Debug().Str("notification", evt[1]).Msg("notification content")
	s.trigger()
}

func (s *subscription) trigger() {
	select {
	case s.c <- struct{}{}:
	default:
	}
}

func (s *subscription) socketioOnError(err error) {
	select {
	case s.sioErrCh <- err:
	default:
	}
}

func (s *subscription) Stop() {
	close(s.closeCh)
	close(s.c)
}
