package main

import (
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"log"
	"time"
)

const (
	STOPPED = iota
	STARTING
	STARTED
	STOPPING
	UNHEALTHY
)

func StatusString(n int) string {
	switch n {
	case STOPPED:
		return "STOPPED"
	case STARTING:
		return "STARTING"
	case STARTED:
		return "STARTED"
	case STOPPING:
		return "STOPPING"
	case UNHEALTHY:
		return "UNHEALTHY"
	default:
		return "INVALIDSTATUS"
	}
}

// Check the status of the instances. Currently checks if they are "ready"; all
// stopped or all started. Will need to be extended to determine actual status.
func (fw *Flywheel) HealthWatcher(out chan<- int) {
	out <- fw.CheckAll()

	ticker := time.NewTicker(fw.hcInterval)
	for {
		select {
		case <-ticker.C:
			out <- fw.CheckAll()
		}
	}
}

func (fw *Flywheel) CheckAll() int {
	health := make(map[string]int)

	err := fw.CheckInstances(health)
	if err != nil {
		log.Print(err)
		return UNHEALTHY
	}

	err = fw.CheckStoppedAutoScalingGroups(health)
	if err != nil {
		log.Print(err)
		return UNHEALTHY
	}

	_, terminated := health["terminated"]
	_, starting := health["pending"]
	_, stopping := health["stopping"]
	_, shutting := health["shutting-down"]
	_, running := health["running"]
	_, stopped := health["stopped"]

	switch {
	case starting && (stopping || shutting):
		return UNHEALTHY

	case running && stopped:
		return UNHEALTHY

	case terminated:
		log.Print("Instance terminated, manual intervention required")
		return UNHEALTHY

	case starting:
		return STARTING

	case stopping, shutting:
		return STOPPING

	case running:
		return STARTED

	case stopped:
		return STOPPED

	default:
		return UNHEALTHY
	}
}

func (fw *Flywheel) CheckInstances(health map[string]int) error {
	resp, err := fw.ec2.DescribeInstances(
		&ec2.DescribeInstancesInput{
			InstanceIds: fw.config.AwsInstances(),
		},
	)
	if err != nil {
		log.Print(err)
		return err
	}

	for _, reservation := range resp.Reservations {
		for _, instance := range reservation.Instances {
			state := *instance.State.Name
			health[state] = health[state] + 1
		}
	}

	return nil
}

func (fw *Flywheel) CheckStoppedAutoScalingGroups(health map[string]int) error {
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
		log.Print(err)
		return err
	}

	for _, group := range resp.AutoScalingGroups {
		healthy := true
		for _, instance := range group.Instances {
			if *instance.HealthStatus != "Healthy" {
				healthy = false
				break
			}
		}

		if healthy {
			health["running"] += 1
			if len(group.SuspendedProcesses) > 0 {
				_, err = fw.autoscaling.ResumeProcesses(
					&autoscaling.ScalingProcessQuery{
						AutoScalingGroupName: group.AutoScalingGroupName,
					},
				)
				if err != nil {
					return err
				}
			}
		} else {
			health["stopped"] += 1
		}
	}

	return nil
}
