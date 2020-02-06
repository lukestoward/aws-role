// Copyright © 2018 Ryan French <ryan@ryanfrench.co>

package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	roleArn  string
	duration int
)

var rootCmd = &cobra.Command{
	Use:   "aws-role --role-arn=[role] [command]",
	Short: "Assume a role in AWS and optionally run a command",
	Long: `
Run a command within the context of assuming a role. This is not persistent, and will only affect the command that is passed in.

e.g.

aws-role --role-arn=arn:aws:iam::1234567890:role/my-role aws s3 ls`,
	Run:                   run,
	Version:               "0.2.0",
	Args:                  cobra.MinimumNArgs(1),
	DisableFlagParsing:    true,
	DisableFlagsInUseLine: true,
	PersistentPreRun:      preRun,
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	_, err := exec.LookPath("aws")
	if err != nil {
		log.WithError(err).Fatal("aws cli is not installed. For information on how to install the aws cli, please visit https://aws.amazon.com/cli/")
	}
	if err := rootCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}

func preRun(cmd *cobra.Command, args []string) {
	var index int
	for i, arg := range args {
		if strings.HasPrefix(arg, "--") {
			cmd.Flags().Parse([]string{arg})
		} else {
			index = i
			break
		}
	}
	args = args[index:]
	cmd.SetArgs(args)
}

func stripFlags(args []string) []string {
	var index int
	for i, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			index = i
			break
		}
	}
	args = args[index:]
	return args
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.Flags().StringVarP(&roleArn, "role-arn", "r", "", "The arn of the role to assume in AWS (required)")
	rootCmd.MarkFlagRequired("role-arn")

	rootCmd.Flags().IntVarP(&duration, "duration", "d", 3600, "The duration, in seconds, for the role to be assumed")
}

func run(cmd *cobra.Command, args []string) {
	args = stripFlags(args)

	// Duration max is 12 hours
	if duration > 43200 || duration < 1 {
		log.
			WithField("command", cmd.Args).
			WithError(errors.New("Duration cannot be longer than 12 hours (43200 seconds) or less than 1 second")).
			Fatalln("Failed to run command")
	}

	roleSessionName, _ := uuid.NewUUID()
	svc := sts.New(session.New())
	input := &sts.AssumeRoleInput{
		DurationSeconds: aws.Int64(3600),
		RoleArn:         aws.String(roleArn),
		RoleSessionName: aws.String(roleSessionName.String()),
	}

	assumeRoleResponse, err := svc.AssumeRole(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case sts.ErrCodeMalformedPolicyDocumentException:
				log.WithError(aerr).
					Errorln(sts.ErrCodeMalformedPolicyDocumentException)
			case sts.ErrCodePackedPolicyTooLargeException:
				log.WithError(aerr).
					Errorln(sts.ErrCodePackedPolicyTooLargeException)
			case sts.ErrCodeRegionDisabledException:
				log.WithError(aerr).
					Errorln(sts.ErrCodeRegionDisabledException)
			default:
				log.WithError(aerr).
					Errorln(sts.ErrCodeRegionDisabledException)
			}
		} else {
			log.WithError(err).
				Errorln("Error assuming role")
		}
		return
	}

	command := exec.Command(args[0], args[1:]...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = append(os.Environ(),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", *assumeRoleResponse.Credentials.AccessKeyId),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", *assumeRoleResponse.Credentials.SecretAccessKey),
		fmt.Sprintf("AWS_SESSION_TOKEN=%s", *assumeRoleResponse.Credentials.SessionToken))

	if err := command.Run(); err != nil {
		log.
			WithField("command", command.Args).
			WithError(err).
			Fatalln("Failed to run command")
	}
}

func initConfig() {
	viper.AutomaticEnv() // read in environment variables that match
}
