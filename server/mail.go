package server

import (
	"log"

	"github.com/xhit/go-simple-mail/v2"
)

func smtpConnect() (*mail.SMTPClient, error) {
	config := Config.Mail

	server := mail.NewSMTPClient()
	server.Host = config.Server
	server.Port = 587
	server.Username = config.Username
	server.Password = config.Password
	server.Encryption = mail.EncryptionSTARTTLS
	return server.Connect()
}

func SendError(subj string, body string) {
	from := "independ <info@independ.org>"
	to := Config.Mail.ErrorTo
	email := mail.NewMSG()
	email.SetFrom(from).AddTo(to).SetSubject(subj)
	email.SetBody(mail.TextHTML, "<pre>"+body+"</pre>")

	if email.Error != nil {
		log.Println("error creating error email:", email.Error)
		return
	}

	client, err := smtpConnect()
	if err != nil {
		log.Println("error connecting to server:", err)
	}
	defer client.Close()
	if err = email.Send(client); err != nil {
		log.Println("error sending error email:", err)
	}

	log.Println("error email send:", subj)
}
