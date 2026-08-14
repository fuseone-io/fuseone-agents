package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
Where runs report, configured.

Outbound only, and the endpoints say so: there is nothing here that lets a
conversation start anything. What a connection grants is the ability to be
spoken to, which is why configuring one needs the same permission as
configuring a model provider and no more.
*/

// ChannelAdmin is channel configuration, declared here by the consumer.
type ChannelAdmin interface {
	List(ctx context.Context) ([]admin.Channel, error)
	PutChannel(ctx context.Context, ch admin.Channel, creds channel.Credentials, by domain.UserID) error
	DeleteChannel(ctx context.Context, name string, by domain.UserID) error
	PutConversation(ctx context.Context, channelName string, conv admin.Conversation, by domain.UserID) error
	DeleteConversation(ctx context.Context, id string, scope domain.Scope, by domain.UserID) error
}

// Announcer posts one message, for proving the wiring.
type Announcer interface {
	Post(ctx context.Context, c channel.Conversation, m channel.Message) (string, error)
}

// WithChannels wires channel configuration and the driver that proves it.
func (s *Server) WithChannels(channels ChannelAdmin, announcer Announcer) *Server {
	s.channels, s.announcer = channels, announcer
	return s
}

func (s *Server) ListChannels(
	ctx context.Context, _ openapi.ListChannelsRequestObject,
) (openapi.ListChannelsResponseObject, error) {
	if resp := s.refuse(ctx, permConfigure); resp != nil {
		return openapi.ListChannels403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	// The vendors come from the process, not from the screen. A console that
	// decided for itself which kinds exist would offer one the binary cannot
	// build, and the failure would arrive as a saved connection that never
	// delivers.
	kinds := []string{}
	if s.channelListing != nil {
		kinds = s.channelListing.Kinds()
	}
	if s.channels == nil {
		return openapi.ListChannels200JSONResponse{
			Items: []openapi.Channel{}, Kinds: kinds,
		}, nil
	}

	configured, err := s.channels.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}

	items := make([]openapi.Channel, 0, len(configured))
	for _, c := range configured {
		items = append(items, channelFrom(c))
	}
	return openapi.ListChannels200JSONResponse{Items: items, Kinds: kinds}, nil
}

func (s *Server) PutChannel(
	ctx context.Context, req openapi.PutChannelRequestObject,
) (openapi.PutChannelResponseObject, error) {
	caller, resp := s.configurer(ctx)
	if resp != nil {
		return openapi.PutChannel403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.channels == nil || req.Body == nil {
		return nil, errNoAdministration
	}

	err := s.channels.PutChannel(ctx, admin.Channel{
		Name:      req.Name,
		Kind:      string(req.Body.Kind),
		Workspace: valueOr(req.Body.Workspace),
		Enabled:   orDefault(req.Body.Enabled, true),
	}, channel.Credentials{
		Token:   valueOr(req.Body.Token),
		Signing: valueOr(req.Body.SigningSecret),
	}, caller)
	if err != nil {
		return openapi.PutChannel400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}
	return openapi.PutChannel204Response{}, nil
}

func (s *Server) DeleteChannel(
	ctx context.Context, req openapi.DeleteChannelRequestObject,
) (openapi.DeleteChannelResponseObject, error) {
	caller, resp := s.configurer(ctx)
	if resp != nil {
		return openapi.DeleteChannel403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.channels == nil {
		return nil, errNoAdministration
	}
	if err := s.channels.DeleteChannel(ctx, req.Name, caller); err != nil {
		return nil, fmt.Errorf("delete channel: %w", err)
	}
	return openapi.DeleteChannel204Response{}, nil
}

func (s *Server) PutConversation(
	ctx context.Context, req openapi.PutConversationRequestObject,
) (openapi.PutConversationResponseObject, error) {
	caller, resp := s.configurer(ctx)
	if resp != nil {
		return openapi.PutConversation403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.channels == nil || req.Body == nil {
		return nil, errNoAdministration
	}

	err := s.channels.PutConversation(ctx, req.Name, admin.Conversation{
		ID:    req.Conversation,
		Label: valueOr(req.Body.Label),
		Scope: domain.Scope{
			Company: domain.CompanyID(req.Body.Company),
			Area:    domain.AreaID(valueOr(req.Body.Area)),
		},
		Wants:   wantsOf(req.Body.Wants),
		Enabled: orDefault(req.Body.Enabled, true),
	}, caller)
	if err != nil {
		return openapi.PutConversation400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}
	return openapi.PutConversation204Response{}, nil
}

func (s *Server) DeleteConversation(
	ctx context.Context, req openapi.DeleteConversationRequestObject,
) (openapi.DeleteConversationResponseObject, error) {
	caller, resp := s.configurer(ctx)
	if resp != nil {
		return openapi.DeleteConversation403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.channels == nil {
		return nil, errNoAdministration
	}

	// The scope comes from what is stored rather than from the request: a
	// caller naming a scope could delete a conversation belonging to another.
	scope, found := s.scopeOfConversation(ctx, req.Name, req.Conversation)
	if !found {
		return openapi.DeleteConversation204Response{}, nil
	}
	if err := s.channels.DeleteConversation(ctx, req.Conversation, scope, caller); err != nil {
		return nil, fmt.Errorf("delete conversation: %w", err)
	}
	return openapi.DeleteConversation204Response{}, nil
}

func (s *Server) scopeOfConversation(
	ctx context.Context, channelName, id string,
) (domain.Scope, bool) {
	configured, err := s.channels.List(ctx)
	if err != nil {
		return domain.Scope{}, false
	}
	for _, c := range configured {
		if c.Name != channelName {
			continue
		}
		for _, conv := range c.Conversations {
			if conv.ID == id {
				return conv.Scope, true
			}
		}
	}
	return domain.Scope{}, false
}

func channelFrom(c admin.Channel) openapi.Channel {
	out := openapi.Channel{
		Name: c.Name, Kind: c.Kind, Enabled: c.Enabled,
		HasCredential: c.HasCredential, HasSigning: ptr(c.HasSigning),
		Conversations: make([]openapi.ChannelConversation, 0, len(c.Conversations)),
	}
	if c.Workspace != "" {
		out.Workspace = ptr(c.Workspace)
	}
	for _, conv := range c.Conversations {
		item := openapi.ChannelConversation{
			Id: conv.ID, Enabled: conv.Enabled,
			Scope: openapi.Scope{
				Company: string(conv.Scope.Company), Area: string(conv.Scope.Area),
			},
		}
		if conv.Label != "" {
			item.Label = ptr(conv.Label)
		}
		if len(conv.Wants) > 0 {
			item.Wants = &conv.Wants
		}
		out.Conversations = append(out.Conversations, item)
	}
	return out
}

func wantsOf(wants *[]openapi.PutConversationJSONBodyWants) []string {
	if wants == nil {
		return nil
	}
	out := make([]string, 0, len(*wants))
	for _, w := range *wants {
		out = append(out, string(w))
	}
	return out
}

// orDefault reads an optional body field, falling back to what the contract
// declares as its default. Distinct from valueOr, which gives the zero value:
// `enabled` absent means true, and a zero there would switch off every channel
// configured by a client that left the field out.
func orDefault[T any](v *T, fallback T) T {
	if v == nil {
		return fallback
	}
	return *v
}

/*
TestConversation posts one message saying what it is.

Configuring a channel otherwise means waiting for a run to park to discover
whether the bot was ever invited — and a notification that silently goes
nowhere is the failure this whole stage exists to avoid. The refusal carries
the channel's own word for what went wrong, because "not_in_channel" tells an
operator what to do and "post failed" tells them to go and find out.
*/
func (s *Server) TestConversation(
	ctx context.Context, req openapi.TestConversationRequestObject,
) (openapi.TestConversationResponseObject, error) {
	if _, resp := s.configurer(ctx); resp != nil {
		return openapi.TestConversation403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.channels == nil || s.announcer == nil {
		return nil, errNoAdministration
	}

	scope, found := s.scopeOfConversation(ctx, req.Name, req.Conversation)
	if !found {
		return openapi.TestConversation400ApplicationProblemPlusJSONResponse(
			invalid("no such conversation is configured")), nil
	}

	ref, err := s.announcer.Post(ctx,
		channel.Conversation{Channel: req.Name, ID: req.Conversation},
		channel.Message{Event: eventTest, Scope: scope})
	if err != nil {
		return openapi.TestConversation400ApplicationProblemPlusJSONResponse(
			invalid(err.Error())), nil
	}
	return openapi.TestConversation200JSONResponse{Delivered: true, Ref: &ref}, nil
}

// eventTest is not one of the events a run produces. It exists so the driver
// renders a message that says it is a test rather than one that reads like a
// run nobody can find.
const eventTest channel.Event = "test"

// Lister answers which conversations a connection can be pointed at, and which
// vendors this binary can connect at all.
type Lister interface {
	Conversations(ctx context.Context, channel string) ([]channel.Available, error)
	Kinds() []string
}

// WithChannelListing wires the picker's source.
func (s *Server) WithChannelListing(l Lister) *Server {
	s.channelListing = l
	return s
}

/*
ListAvailableConversations offers the places the bot is already in.

Nobody should have to find `C0123ABCDEF`. What an operator knows is `#alertas`,
so that is what they pick — and because the listing is what the bot is a member
of, the picker cannot offer somewhere the message would fail to arrive.
*/
func (s *Server) ListAvailableConversations(
	ctx context.Context, req openapi.ListAvailableConversationsRequestObject,
) (openapi.ListAvailableConversationsResponseObject, error) {
	if resp := s.refuse(ctx, permConfigure); resp != nil {
		return openapi.ListAvailableConversations403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.channelListing == nil {
		return nil, errNoAdministration
	}

	found, err := s.channelListing.Conversations(ctx, req.Name)
	if err != nil {
		// The channel's own reason. An app granted chat:write and not
		// channels:read can post and cannot list, and an empty answer there
		// would read as "the bot is in no channels" — which sends somebody to
		// fix the wrong thing entirely.
		return openapi.ListAvailableConversations400ApplicationProblemPlusJSONResponse(
			invalid(err.Error())), nil
	}

	items := make([]struct {
		Id      string `json:"id"`
		Name    string `json:"name"`
		Private *bool  `json:"private,omitempty"`
	}, 0, len(found))
	for _, c := range found {
		private := c.Private
		items = append(items, struct {
			Id      string `json:"id"`
			Name    string `json:"name"`
			Private *bool  `json:"private,omitempty"`
		}{Id: c.ID, Name: c.Name, Private: &private})
	}
	return openapi.ListAvailableConversations200JSONResponse{Items: items}, nil
}
