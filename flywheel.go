package main

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"log"
	"time"
)

const SPIN_INTERVAL = 15 * time.Second

type Ping struct {
	replyTo      chan int
	requestStart bool
	requestStop  bool
}

type Flywheel struct {
	config      *Config
	running     bool
	pings       chan Ping
	status      int
	ready       bool
	stopAt      time.Time
	ec2         *ec2.EC2
	autoscaling *autoscaling.AutoScaling
	hcInterval  time.Duration
	idleTimeout time.Duration
}

func New(config *Config) *Flywheel {
	region := "ap-southeast-2"

	var hcInterval time.Duration
	var idleTimeout time.Duration

	s := config.HcInterval
	if s == "" {
		hcInterval = time.Minute
	} else {
		d, err := time.ParseDuration(s)
		if err != nil {
			log.Printf("Invalid duration: %v", err)
			hcInterval = time.Minute
		} else {
			hcInterval = d
		}
	}

	s = config.IdleTimeout
	if s == "" {
		idleTimeout = time.Minute
	} else {
		d, err := time.ParseDuration(s)
		if err != nil {
			log.Printf("Invalid duration: %v", err)
			idleTimeout = time.Minute
		} else {
			idleTimeout = d
		}
	}

	awsConfig := &aws.Config{Region: &region}
	return &Flywheel{
		hcInterval:  hcInterval,
		idleTimeout: idleTimeout,
		config:      config,
		pings:       make(chan Ping),
		stopAt:      time.Now(),
		ec2:         ec2.New(awsConfig),
		autoscaling: autoscaling.New(awsConfig),
	}
}

func (fw *Flywheel) Spin() {
	hchan := make(chan int, 1)

	go fw.HealthWatcher(hchan)

	ticker := time.NewTicker(SPIN_INTERVAL)
	for {
		select {
		case ping := <-fw.pings:
			fw.RecvPing(&ping)
		case <-ticker.C:
			fw.Poll()
		case status := <-hchan:
			if fw.status != status {
				log.Printf("Healthcheck - status is now %v", StatusString(status))
				fw.status = status
			}
		}
	}
}

func (fw *Flywheel) RecvPing(ping *Ping) {
	ch := ping.replyTo
	defer close(ch)

	switch fw.status {
	case STOPPED:
		if ping.requestStart {
			fw.Start()
		}

	case STARTED:
		if ping.requestStop {
			fw.Stop()
		} else {
			fw.stopAt = time.Now().Add(fw.idleTimeout)
			log.Printf("Timer update. Stop scheduled for %v", fw.stopAt)
		}
	}

	ch <- fw.status
}

func (fw *Flywheel) Poll() {
	switch fw.status {
	case STARTED:
		if time.Now().After(fw.stopAt) {
			fw.Stop()
			log.Print("Idle timeout - shutting down")
			fw.status = STOPPING
		}

	case STOPPING:
		if fw.ready {
			log.Print("Shutdown complete")
			fw.status = STOPPED
		}

	case STARTING:
		if fw.ready {
			fw.status = STARTED
			fw.stopAt = time.Now().Add(fw.idleTimeout)
			log.Printf("Startup complete. Stop scheduled for %v", fw.stopAt)
		}
	}
}

func (fw *Flywheel) Start() {
	log.Print("Startup beginning")

	var err error
	err = fw.StartInstances()

	if err == nil {
		err = fw.UnterminateAutoScaling()
	}
	if err == nil {
		err = fw.StartAutoScaling()
	}

	if err != nil {
		log.Printf("Error starting: %v", err)
	} else {
		fw.ready = false
		fw.stopAt = time.Now().Add(fw.idleTimeout)
		fw.status = STARTING
	}
}

func (fw *Flywheel) StartInstances() error {
	_, err := fw.ec2.StartInstances(
		&ec2.StartInstancesInput{
			InstanceIds: fw.config.AwsInstances(),
		},
	)
	return err
}

func (fw *Flywheel) UnterminateAutoScaling() error {
	var err error
	for groupName, size := range fw.config.AutoScaling.Terminate {
		_, err = fw.autoscaling.UpdateAutoScalingGroup(
			&autoscaling.UpdateAutoScalingGroupInput{
				AutoScalingGroupName: &groupName,
				MaxSize:              &size,
				MinSize:              &size,
			},
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (fw *Flywheel) StartAutoScaling() error {
	var err error
	var awsGroupNames []*string
	for _, groupName := range fw.config.AutoScaling.Stop {
		awsGroupNames = append(awsGroupNames, &groupName)
	}

	resp, err := fw.autoscaling.DescribeAutoScalingGroups(
		&autoscaling.DescribeAutoScalingGroupsInput{
			AutoScalingGroupNames: awsGroupNames,
		},
	)
	if err != nil {
		return err
	}

	for _, group := range resp.AutoScalingGroups {
		// NOTE: Processes not unsuspended here. Needs to be triggered after
		// startup, before entering STARTED state.
		instanceIds := []*string{}
		for _, instance := range group.Instances {
			instanceIds = append(instanceIds, instance.InstanceId)
		}

		_, err := fw.ec2.StartInstances(
			&ec2.StartInstancesInput{
				InstanceIds: instanceIds,
			},
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (fw *Flywheel) Stop() {
	var err error
	err = fw.StopInstances()

	if err == nil {
		err = fw.TerminateAutoScaling()
	}
	if err == nil {
		err = fw.StopAutoScaling()
	}

	if err != nil {
		log.Printf("Error stopping: %v", err)
	} else {
		fw.ready = false
		fw.status = STOPPING
	}
}

func (fw *Flywheel) StopInstances() error {
	_, err := fw.ec2.StopInstances(
		&ec2.StopInstancesInput{
			InstanceIds: fw.config.AwsInstances(),
		},
	)
	return err
}

func (fw *Flywheel) StopAutoScaling() error {
	var err error
	var awsGroupNames []*string

	if len(fw.config.AutoScaling.Stop) == 0 {
		return nil
	}

	for _, groupName := range fw.config.AutoScaling.Stop {
		awsGroupNames = append(awsGroupNames, &groupName)
	}

	resp, err := fw.autoscaling.DescribeAutoScalingGroups(
		&autoscaling.DescribeAutoScalingGroupsInput{
			AutoScalingGroupNames: awsGroupNames,
		},
	)
	if err != nil {
		return err
	}

	for _, group := range resp.AutoScalingGroups {
		_, err = fw.autoscaling.SuspendProcesses(
			&autoscaling.ScalingProcessQuery{
				AutoScalingGroupName: group.AutoScalingGroupName,
				ScalingProcesses: []*string{
					aws.String("ReplaceUnhealthy"),
				},
			},
		)
		if err != nil {
			return err
		}

		instanceIds := []*string{}
		for _, instance := range group.Instances {
			instanceIds = append(instanceIds, instance.InstanceId)
		}

		_, err := fw.ec2.StopInstances(
			&ec2.StopInstancesInput{
				InstanceIds: instanceIds,
			},
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (fw *Flywheel) TerminateAutoScaling() error {
	var err error
	var zero int64
	for groupName := range fw.config.AutoScaling.Terminate {
		_, err = fw.autoscaling.UpdateAutoScalingGroup(
			&autoscaling.UpdateAutoScalingGroupInput{
				AutoScalingGroupName: &groupName,
				MaxSize:              &zero,
				MinSize:              &zero,
			},
		)
		if err != nil {
			return err
		}
	}
	return nil
}
