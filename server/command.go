package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gorhill/cronexpr"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/plugin"
)

const deferCommand = "defer-post"
const queueCommand = "messages-queue"

func startMeetingError(channelID string, detailedError string) (*model.CommandResponse, *model.AppError) {
	return &model.CommandResponse{
			ResponseType: model.COMMAND_RESPONSE_TYPE_EPHEMERAL,
			ChannelId:    channelID,
			Text:         "We could not start a meeting at this time.",
		}, &model.AppError{
			Message:       "We could not start a meeting at this time.",
			DetailedError: detailedError,
		}
}

func createDeferCommand() *model.Command {
	return &model.Command{
		Trigger:          deferCommand,
		AutoComplete:     true,
		AutoCompleteDesc: "Defer a message to be sent later.",
		AutoCompleteHint: "[command]",
	}
}

func createQueueCommand() *model.Command {
	return &model.Command{
		Trigger:          queueCommand,
		AutoComplete:     true,
		AutoCompleteDesc: "Manage messages queues. Create/Delete/Modify queues, and add messages to the queue.",
		AutoCompleteHint: "[command]",
	}
}

func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	split := strings.Fields(args.Command)
	command := split[0]

	if command == "/"+deferCommand {
		return p.executeDeferCommand(c, args)
	}

	if command == "/"+queueCommand {
		return p.executeQueueCommand(c, args)
	}
	return &model.CommandResponse{}, nil
}

func (p *Plugin) executeQueueCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	split := strings.Fields(args.Command)
	if (len(split) == 2 && split[1] == "help") || len(split) == 1 {
		return p.executeQueueHelpCommand(c, args)
	}

	if split[1] == "create" {
		if len(split) < 4 {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Not enough arguments to create the queue",
			})
			return &model.CommandResponse{}, nil
		}
		scheduleSpec, err := cronexpr.Parse(strings.Join(split[3:], " "))
		if err != nil {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Unable to parse the schedule, please see the supported format in the help text",
			})
			return &model.CommandResponse{}, nil
		}
		p.Queues[split[2]] = &Queue{
			Name:      split[2],
			Spec:      strings.Join(split[3:], " "),
			ChannelId: args.ChannelId,
			Messages:  []string{},
		}
		_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
			ChannelId: args.ChannelId,
			Message:   fmt.Sprintf("Scheduling a queue, next execution: %v", scheduleSpec.Next(time.Now())),
		})
		return &model.CommandResponse{}, nil
	}

	if split[1] == "list" {
		if len(p.Queues) == 0 {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "No queues defined yet",
			})
			return &model.CommandResponse{}, nil
		}

		queuesList := []string{}
		for _, queue := range p.Queues {
			nextMessage := "not messages in the queue"
			if len(queue.Messages) > 0 {
				nextMessage = queue.Messages[0]
			}
			nextTime := "not able to get the time spec properly"
			scheduleSpec, err := cronexpr.Parse(queue.Spec)
			if err == nil {
				nextTime = fmt.Sprintf("%v", scheduleSpec.Next(time.Now()))
			}
			queuesList = append(queuesList, fmt.Sprintf(" * %s\n  * channel id: %s\n  * schedule spec: %s\n  * next execution: %s\n  * next message: %s",
				queue.Name, queue.ChannelId, queue.Spec, nextTime, nextMessage,
			))
		}

		sort.Slice(queuesList, func(i, j int) bool {
			return queuesList[i] < queuesList[j]
		})

		queuesList = append([]string{"#### List of queues:"}, queuesList...)
		_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
			ChannelId: args.ChannelId,
			Message:   strings.Join(queuesList, "\n"),
		})
		return &model.CommandResponse{}, nil
	}

	if split[1] == "add-message" {
		if len(split) < 4 {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Not enough arguments to add a message",
			})
			return &model.CommandResponse{}, nil
		}
		queue, ok := p.Queues[split[2]]
		if !ok {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   fmt.Sprintf("Unknown queue %s.", split[2]),
			})
			return &model.CommandResponse{}, nil
		}
		queue.Messages = append(queue.Messages, strings.Join(split[3:], " "))
		_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
			ChannelId: args.ChannelId,
			Message:   "Message added to the queue",
		})
		return &model.CommandResponse{}, nil
	}

	if split[1] == "list-messages" {
		if len(split) < 3 {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Not enough arguments to list messages",
			})
			return &model.CommandResponse{}, nil
		}
		queue, ok := p.Queues[split[2]]
		if !ok {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   fmt.Sprintf("Unknown queue %s.", split[2]),
			})
			return &model.CommandResponse{}, nil
		}

		listOfMessages := []string{fmt.Sprintf("#### List of messages for the queue %s:", queue.Name)}
		for _, message := range queue.Messages {
			listOfMessages = append(listOfMessages, fmt.Sprintf(" * %s", message))
		}
		_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
			ChannelId: args.ChannelId,
			Message:   strings.Join(listOfMessages, "\n"),
		})
		return &model.CommandResponse{}, nil
	}
	return &model.CommandResponse{}, nil
}

func (p *Plugin) executeDeferCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	split := strings.Fields(args.Command)
	timeSpec := ""
	if len(split) < 3 {
		if len(split) == 2 && split[1] == "help" {
			return p.executeDeferHelpCommand(c, args)
		}

		return &model.CommandResponse{
				ResponseType: model.COMMAND_RESPONSE_TYPE_EPHEMERAL,
				ChannelId:    args.ChannelId,
				Text:         "Not enough parameters",
			}, &model.AppError{
				Message:       "Not enough parameters",
				DetailedError: "Not enough parameters",
			}
	}

	timeSpec = split[1]
	messageStart := strings.Index(args.Command, timeSpec) + len(timeSpec)
	message := args.Command[messageStart:]

	if timeSpec == "online" {
		channel, appErr := p.API.GetChannel(args.ChannelId)
		if appErr != nil {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Unable to defer the message until the user is online",
			})
			return &model.CommandResponse{}, nil
		}
		if channel.Type != model.CHANNEL_DIRECT {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Unable to defer the message until the user is online in not DMs channels",
			})
			return &model.CommandResponse{}, nil
		}

		members, appErr := p.API.GetChannelMembers(args.ChannelId, 0, 10)
		if appErr != nil {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Unable to defer the message until the user is online",
			})
			return &model.CommandResponse{}, nil
		}

		otherUserId := ""
		for _, member := range *members {
			if member.UserId != args.UserId {
				otherUserId = member.UserId
			}
		}

		fmt.Println("------ ON ADD NEW ------")
		fmt.Println(p.postsWaitingForOnline)
		p.postsWaitingForOnline[otherUserId] = append(p.postsWaitingForOnline[otherUserId], &model.Post{
			UserId:    args.UserId,
			ChannelId: args.ChannelId,
			RootId:    args.RootId,
			ParentId:  args.ParentId,
			Message:   message,
		})
		fmt.Println(p.postsWaitingForOnline)
		return &model.CommandResponse{
			ResponseType: model.COMMAND_RESPONSE_TYPE_EPHEMERAL,
			ChannelId:    args.ChannelId,
			Text:         "Message defered",
		}, nil
	}

	duration, err := time.ParseDuration(timeSpec)
	if err != nil {
		return &model.CommandResponse{
				ResponseType: model.COMMAND_RESPONSE_TYPE_EPHEMERAL,
				ChannelId:    args.ChannelId,
				Text:         "Not valid time format",
			}, &model.AppError{
				Message:       "Not valid time format",
				DetailedError: err.Error(),
			}
	}

	model.CreateTask("defer message", func() {
		_, err := p.API.CreatePost(&model.Post{
			UserId:    args.UserId,
			ChannelId: args.ChannelId,
			RootId:    args.RootId,
			ParentId:  args.ParentId,
			Message:   message,
		})
		if err != nil {
			p.API.LogError(err.Error())
		}
	}, duration)

	return &model.CommandResponse{
		ResponseType: model.COMMAND_RESPONSE_TYPE_EPHEMERAL,
		ChannelId:    args.ChannelId,
		Text:         "Message defered",
	}, nil
}

func (p *Plugin) executeDeferHelpCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	helpTitle := `###### Defer Post - Slash Command help
`
	commandHelp := `* |/defer-post [time] [message]| - Send the message after the time has passed
* |/defer-post online [message]| - Send the message when the user is online (only valid for DMs)
* |/defer-post help| - Show this help text

###### Time format:
* The time can be specified in the golang format that you can see [here](https://golang.org/pkg/time/#ParseDuration)`
	text := helpTitle + strings.Replace(commandHelp, "|", "`", -1)
	post := &model.Post{
		ChannelId: args.ChannelId,
		Message:   text,
	}
	_ = p.API.SendEphemeralPost(args.UserId, post)

	return &model.CommandResponse{}, nil
}

func (p *Plugin) executeQueueHelpCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	helpTitle := `###### Defer Post - Slash Command help
`
	commandHelp := `* |/messages-queue create <name> <schedule>| - Create a queue for the current channel (see the Schedule format help at the bottom)
* |/messages-queue list| - List the queues for this chanell
* |/messages-queue add-message <queue-name> <message>| - Add a new message to the queue
* |/messages-queue list-messages <queue-name>| - Add a new message the the queue
* |/messages-queue remove-message <queue-name> <position>| - Remove a message from the queue in the specified position
* |/messages-queue insert-message <queue-name> <position> <message>| - Add a new message to the queue in the speicified position
* |/messages-queue help| - Show this help text

###### Schedule format:
* The schedule format used is the cron expresion format, you can see more information [here](https://en.wikipedia.org/wiki/Cron)`
	text := helpTitle + strings.Replace(commandHelp, "|", "`", -1)
	post := &model.Post{
		ChannelId: args.ChannelId,
		Message:   text,
	}
	_ = p.API.SendEphemeralPost(args.UserId, post)

	return &model.CommandResponse{}, nil
}