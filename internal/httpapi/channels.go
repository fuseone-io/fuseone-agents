package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

/*
Where runs report, and where deliberate asks can arrive.

The two halves share the connection and not the authority. Posting uses the bot
credential, HTTP inbound uses the signing secret, and Socket Mode uses an
app-level token. What still stays true is that a conversation grants nothing
to an agent: the Gate decides effects after the run starts.
*/

// ChannelAdmin is channel configuration, declared here by the consumer.
type ChannelAdmin interface {
	List(ctx context.Context) ([]admin.Channel, error)
	PutChannel(ctx context.Context, ch admin.Channel, creds channel.Credentials, by domain.UserID) error
	DeleteChannel(ctx context.Context, name string, by domain.UserID) error
	PutConversation(ctx context.Context, channelName string, conv admin.Conversation, by domain.UserID) error
	DeleteConversation(ctx context.Context, id string, scope domain.Scope, by domain.UserID) error
	Identities(ctx context.Context) ([]admin.ChannelIdentity, error)
	SeenAccounts(ctx context.Context) ([]admin.ChannelAccountSeen, error)
	BindIdentity(ctx context.Context, id admin.ChannelIdentity, by domain.UserID) error
	UnbindIdentity(ctx context.Context, channelName, account string, by domain.UserID) error
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

	// One read for every channel rather than one per channel: bindings are few
	// and a listing that queried per row would ask the same question five
	// times to draw five cards.
	bound, err := s.channels.Identities(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channel identities: %w", err)
	}
	seen, err := s.channels.SeenAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list seen channel accounts: %w", err)
	}

	items := make([]openapi.Channel, 0, len(configured))
	for _, c := range configured {
		items = append(items, channelFrom(c, bound, seen))
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

	delivery := channel.DeliveryHTTP
	if req.Body.DeliveryMode != nil {
		delivery = string(*req.Body.DeliveryMode)
	}

	err := s.channels.PutChannel(ctx, admin.Channel{
		Name:         req.Name,
		Kind:         string(req.Body.Kind),
		Workspace:    valueOr(req.Body.Workspace),
		DeliveryMode: delivery,
		Enabled:      orDefault(req.Body.Enabled, true),
	}, channel.Credentials{
		Token:    valueOr(req.Body.Token),
		AppToken: valueOr(req.Body.AppToken),
		Signing:  valueOr(req.Body.SigningSecret),
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
	scope := domain.Scope{
		Company: domain.CompanyID(req.Body.Company),
		Area:    domain.AreaID(valueOr(req.Body.Area)),
	}
	mode := conversationMode(req.Body.Mode)
	agent := domain.AgentID(valueOr(req.Body.Agent))
	runAs := domain.UserID(valueOr(req.Body.RunAs))
	if mode == channel.ConversationWatch {
		if agent == "" {
			return openapi.PutConversation400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
					invalid("watched messages need an agent to start")),
			}, nil
		}
		reason, err := s.refuseWatchAgent(ctx, agent, scope)
		if err != nil {
			return nil, err
		}
		if reason != "" {
			return openapi.PutConversation400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
					invalid(reason)),
			}, nil
		}
		if runAs == "" {
			return openapi.PutConversation400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
					invalid("runAs is required")),
			}, nil
		}
		if resp := s.refuseRunAsDelegation(ctx, caller, runAs); resp != nil {
			return openapi.PutConversation403ApplicationProblemPlusJSONResponse{
				ForbiddenApplicationProblemPlusJSONResponse: *resp,
			}, nil
		}
		reason, err = s.refuseRunAsPrincipal(ctx, runAs, scope)
		if err != nil {
			return nil, err
		}
		if reason != "" {
			return openapi.PutConversation400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
					invalid(reason)),
			}, nil
		}
	}

	err := s.channels.PutConversation(ctx, req.Name, admin.Conversation{
		ID:      req.Conversation,
		Label:   valueOr(req.Body.Label),
		Scope:   scope,
		Mode:    mode,
		Sources: valueOrSlice(req.Body.Sources),
		Agent:   agent,
		RunAs:   runAs,
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

func (s *Server) refuseWatchAgent(
	ctx context.Context, agent domain.AgentID, scope domain.Scope,
) (string, error) {
	if s.agents == nil {
		return "", nil
	}
	/*
		Watched messages name the agent in configuration rather than in Slack
		text, but the same two facts still have to intersect: this is the scope
		the conversation speaks for, and the published version declared that a
		conversation may start it. Without this check, a client can save a
		configuration that only fails later, in the Slack thread.
	*/
	published, err := s.agents.List(ctx, scope, false)
	if err != nil {
		return "", fmt.Errorf("list agents startable from channel: %w", err)
	}
	for _, one := range published {
		if one.ID != agent {
			continue
		}
		for _, trigger := range one.Triggers {
			if trigger.Type == spec.TriggerChannel {
				return "", nil
			}
		}
		return "watched messages need an agent that declares the Conversation trigger in this scope", nil
	}
	return "watched messages need an agent published in this scope", nil
}

func (s *Server) refuseRunAsDelegation(
	ctx context.Context, caller, runAs domain.UserID,
) *openapi.ForbiddenApplicationProblemPlusJSONResponse {
	if runAs == "" || runAs == caller {
		return nil
	}
	/*
		Naming somebody else here is delegation.

		For watched messages that principal becomes OnBehalfOf, and personal MCP
		credentials are keyed by that exact principal. A channel configurer may
		configure where messages arrive, but they may not turn another person's
		GitHub, Google or Slack credential into the identity of an automated run.
		That is identity administration, so it uses the same installation-wide
		scope as People and identity-provider mappings.
	*/
	if err := auth.Require(ctx, domain.PermIdentityWrite, identityScope); err != nil {
		body := forbidden(domain.PermIdentityWrite, identityScope)
		return &body
	}
	return nil
}

func (s *Server) refuseRunAsPrincipal(
	ctx context.Context, runAs domain.UserID, scope domain.Scope,
) (string, error) {
	if runAs == "" || s.people == nil {
		return "", nil
	}
	found, err := s.people.People(ctx)
	if err != nil {
		return "", fmt.Errorf("list people for runAs: %w", err)
	}
	for _, person := range found {
		if person.ID == string(runAs) {
			if person.Disabled {
				return "runAs principal is not known or is disabled", nil
			}
			for _, grant := range person.Grants {
				if grant.Scope.Contains(scope) {
					return "", nil
				}
			}
			return "runAs principal has no grant in this scope", nil
		}
	}
	return "runAs principal is not known or is disabled", nil
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

func channelFrom(
	c admin.Channel, bound []admin.ChannelIdentity, seen []admin.ChannelAccountSeen,
) openapi.Channel {
	delivery := openapi.ChannelDeliveryMode(channel.DeliveryMode(c.DeliveryMode))
	out := openapi.Channel{
		Name: c.Name, Kind: c.Kind, Enabled: c.Enabled,
		DeliveryMode:  &delivery,
		HasCredential: c.HasCredential, HasSigning: ptr(c.HasSigning),
		HasAppToken:   ptr(c.HasAppToken),
		Conversations: make([]openapi.ChannelConversation, 0, len(c.Conversations)),
	}
	if c.Workspace != "" {
		out.Workspace = ptr(c.Workspace)
	}
	identities := make([]openapi.ChannelIdentity, 0)
	for _, id := range bound {
		if id.Channel != c.Name {
			continue
		}
		item := openapi.ChannelIdentity{
			Account: id.Account, Principal: string(id.Principal),
		}
		if id.Display != "" {
			item.Display = ptr(id.Display)
		}
		if id.Unreadable {
			// Shown as broken rather than hidden. The runtime refuses an ask
			// on this row and names it, and an operator sent to fix it needs
			// to be able to see it.
			item.Unreadable = ptr(true)
		}
		identities = append(identities, item)
	}
	out.Identities = &identities
	boundHere := make(map[string]struct{}, len(identities))
	for _, id := range identities {
		boundHere[id.Account] = struct{}{}
	}
	seenAccounts := make([]openapi.ChannelSeenAccount, 0)
	for _, account := range seen {
		if account.Channel != c.Name {
			continue
		}
		if _, alreadyBound := boundHere[account.Account]; alreadyBound {
			continue
		}
		item := openapi.ChannelSeenAccount{
			Account: account.Account, LastSeen: account.LastSeen,
		}
		if account.Conversation != "" {
			item.Conversation = ptr(account.Conversation)
		}
		seenAccounts = append(seenAccounts, item)
	}
	out.SeenAccounts = &seenAccounts

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
		mode := openapi.ChannelConversationMode(channel.ConversationMode(conv.Mode))
		item.Mode = &mode
		if len(conv.Sources) > 0 {
			item.Sources = &conv.Sources
		}
		if conv.Agent != "" {
			item.Agent = ptr(string(conv.Agent))
		}
		if conv.RunAs != "" {
			item.RunAs = ptr(string(conv.RunAs))
		}
		if len(conv.Wants) > 0 {
			item.Wants = &conv.Wants
		}
		out.Conversations = append(out.Conversations, item)
	}
	return out
}

func conversationMode(mode *openapi.PutConversationJSONBodyMode) string {
	if mode == nil {
		return channel.ConversationMentions
	}
	return string(*mode)
}

func valueOrSlice(v *[]string) []string {
	if v == nil {
		return nil
	}
	return *v
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

func (s *Server) BindChannelIdentity(
	ctx context.Context, req openapi.BindChannelIdentityRequestObject,
) (openapi.BindChannelIdentityResponseObject, error) {
	caller, resp := s.configurer(ctx)
	if resp != nil {
		return openapi.BindChannelIdentity403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.channels == nil || req.Body == nil {
		return nil, errNoAdministration
	}

	err := s.channels.BindIdentity(ctx, admin.ChannelIdentity{
		Channel:   req.Name,
		Account:   req.Account,
		Principal: domain.UserID(req.Body.Principal),
		Display:   s.displayOf(ctx, req.Body.Principal),
	}, caller)
	if err != nil {
		return openapi.BindChannelIdentity400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}
	return openapi.BindChannelIdentity204Response{}, nil
}

func (s *Server) UnbindChannelIdentity(
	ctx context.Context, req openapi.UnbindChannelIdentityRequestObject,
) (openapi.UnbindChannelIdentityResponseObject, error) {
	caller, resp := s.configurer(ctx)
	if resp != nil {
		return openapi.UnbindChannelIdentity403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.channels == nil {
		return nil, errNoAdministration
	}
	if err := s.channels.UnbindIdentity(ctx, req.Name, req.Account, caller); err != nil {
		return nil, fmt.Errorf("unbind channel identity: %w", err)
	}
	return openapi.UnbindChannelIdentity204Response{}, nil
}

/*
displayOf is who the binding is to, for the screen.

Read at binding time and stored beside it, so the list stays readable when the
directory is unreachable and so a renamed person does not silently become a
different one on screen. It is a convenience and never the key: everything that
decides anything uses the principal.
*/
func (s *Server) displayOf(ctx context.Context, principal string) string {
	if s.people == nil {
		return ""
	}
	found, err := s.people.People(ctx)
	if err != nil {
		return ""
	}
	for _, person := range found {
		if string(person.ID) == principal {
			return person.Display
		}
	}
	return ""
}
