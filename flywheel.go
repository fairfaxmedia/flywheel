package main

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"log"
	"time"
)

// How often flywheel will update its internal state and/or check for idle
// timeouts
const SPIN_INTERVAL = time.Second

// HTTP requests "ping" the flywheel goroutine. This updates the idle timeout,
// and returns the current status to the http request.
type Ping struct {
	replyTo      chan Pong
	requestStart bool
	requestStop  bool
	noop         bool
}

type Pong struct {
	Status      int       `json:"-"`
	StatusName  string    `json:"status"`
	Err         error     `json:"error,omitempty"`
	LastStarted time.Time `json:"last-started,omitempty"`
	LastStopped time.Time `json:"last-stopped,omitempty"`
}

// The Flywheel struct holds all the state required by the flywheel goroutine.
type Flywheel struct {
	config      *Config
	running     bool
	pings       chan Ping
	status      int
	ready       bool
	stopAt      time.Time
	lastStarted time.Time
	lastStopped time.Time
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

// Runs the main loop for the Flywheel.
// Never returns, so should probably be run as a goroutine.
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
				if status == STARTED {
					fw.stopAt = time.Now().Add(fw.idleTimeout)
					log.Printf("Timer update. Stop scheduled for %v", fw.stopAt)
				}
				fw.status = status
			}
		}
	}
}

// HTTP requests "ping" the flywheel goroutine. This updates the idle timeout,
// and returns the current status to the http request.
func (fw *Flywheel) RecvPing(ping *Ping) {
	var pong Pong

	ch := ping.replyTo
	defer close(ch)

	switch fw.status {
	case STOPPED:
		if ping.requestStart {
			pong.Err = fw.Start()
		}

	case STARTED:
		if ping.requestStop {
			pong.Err = fw.Stop()
		} else if ping.noop {
			// Status requests, etc. Don't update idle timer
		} else {
			fw.stopAt = time.Now().Add(fw.idleTimeout)
			log.Printf("Timer update. Stop scheduled for %v", fw.stopAt)
		}
	}

	pong.Status = fw.status
	pong.StatusName = StatusString(fw.status)
	pong.LastStarted = fw.lastStarted
	pong.LastStopped = fw.lastStopped

	ch <- pong
}

// The periodic check for starting/stopping state transitions and idle
// timeouts
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

// Start all the resources managed by the flywheel.
func (fw *Flywheel) Start() error {
	fw.lastStarted = time.Now()
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
		return err
	}

	fw.ready = false
	fw.stopAt = time.Now().Add(fw.idleTimeout)
	fw.status = STARTING
	return nil
}

// Start EC2 instances
func (fw *Flywheel) StartInstances() error {
	if len(fw.config.Instances) == 0 {
		return nil
	}
	log.Printf("Starting instances %v", fw.config.Instances)
	_, err := fw.ec2.StartInstances(
		&ec2.StartInstancesInput{
			InstanceIds: fw.config.AwsInstances(),
		},
	)
	return err
}

// Restore autoscaling group instances
func (fw *Flywheel) UnterminateAutoScaling() error {
	var err error
	for groupName, size := range fw.config.AutoScaling.Terminate {
		log.Printf("Restoring autoscaling group %s", groupName)
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

// Start EC2 instances in a suspended autoscale group
// @note The autoscale group isn't unsuspended here. It's done by the
//       healthcheck once all the instances are healthy.
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
		log.Printf("Starting autoscaling group %s", group.AutoScalingGroupName)
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

// Stop all resources managed by the flywheel
func (fw *Flywheel) Stop() error {
	fw.lastStopped = time.Now()

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
		return err
	}

	fw.ready = false
	fw.status = STOPPING
	return nil
}

// Stop EC2 instances
func (fw *Flywheel) StopInstances() error {
	if len(fw.config.Instances) == 0 {
		return nil
	}
	log.Printf("Stopping instances %v", fw.config.Instances)
	_, err := fw.ec2.StopInstances(
		&ec2.StopInstancesInput{
			InstanceIds: fw.config.AwsInstances(),
		},
	)
	return err
}

// Suspend ReplaceUnhealthy in an autoscale group and stop the instances.
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
		log.Printf("Stopping autoscaling group %s", group.AutoScalingGroupName)

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

// Reduce autoscaling min/max instances to 0, causing the instances to be terminated.
func (fw *Flywheel) TerminateAutoScaling() error {
	var err error
	var zero int64
	for groupName := range fw.config.AutoScaling.Terminate {
		log.Printf("Terminating autoscaling group %s", groupName)
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
