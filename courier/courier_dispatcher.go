package courier

import (
	"context"

	"github.com/pkg/errors"
)

func (c *courier) DispatchMessage(ctx context.Context, msg Message) error {
	messageStatus := MessageStatusSent
	logMessage := "Courier sent out message."

	switch msg.Type {
	case MessageTypeEmail:
		if err := c.dispatchEmail(ctx, msg); err != nil {
			return err
		}
	case MessageTypePhone:
		if err := c.dispatchSMS(ctx, msg); err != nil {
			if m, ok := err.(*MessageRejectedError); ok {
				messageStatus = MessageStatusRejected
				logMessage = m.Error()
			} else {
				return err
			}
		}
	default:
		return errors.Errorf("received unexpected message type: %d", msg.Type)
	}

	if err := c.deps.CourierPersister().SetMessageStatus(ctx, msg.ID, messageStatus); err != nil {
		c.deps.Logger().
			WithError(err).
			WithField("message_id", msg.ID).
			Error(`Unable to set the message status to "sent".`)
		return err
	}

	c.deps.Logger().
		WithField("message_id", msg.ID).
		WithField("message_type", msg.Type).
		WithField("message_template_type", msg.TemplateType).
		WithField("message_subject", msg.Subject).
		Debug(logMessage)

	return nil
}

func (c *courier) DispatchQueue(ctx context.Context) error {
	messages, err := c.deps.CourierPersister().NextMessages(ctx, 10)
	if err != nil {
		if errors.Is(err, ErrQueueEmpty) {
			return nil
		}
		return err
	}

	for k := range messages {
		var msg = messages[k]
		if err := c.DispatchMessage(ctx, msg); err != nil {
			for _, replace := range messages[k:] {
				if err := c.deps.CourierPersister().SetMessageStatus(ctx, replace.ID, MessageStatusQueued); err != nil {
					if c.failOnError {
						return err
					}
					c.deps.Logger().
						WithError(err).
						WithField("message_id", replace.ID).
						Error(`Unable to reset the failed message's status to "queued".`)
				}
			}

			return err
		}
	}

	return nil
}
