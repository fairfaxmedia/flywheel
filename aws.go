package flywheel

import (
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// Start all the resources managed by the flywheel.
func (fw *Flywheel) Start() error {
	fw.lastStarted = time.Now()
	log.Print("Startup beginning")

	var err error
	err = fw.startInstances()

	if err == nil {
		err = fw.unterminateAutoScaling()
	}
	if err == nil {
		err = fw.startAutoScaling()
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
func (fw *Flywheel) startInstances() error {
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

// UnterminateAutoScaling - Restore autoscaling group instances
func (fw *Flywheel) unterminateAutoScaling() error {
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
func (fw *Flywheel) startAutoScaling() error {
	for _, groupName := range fw.config.AutoScaling.Stop {
		log.Printf("Starting autoscaling group %s", groupName)

		resp, err := fw.autoscaling.DescribeAutoScalingGroups(
			&autoscaling.DescribeAutoScalingGroupsInput{
				AutoScalingGroupNames: []*string{&groupName},
			},
		)
		if err != nil {
			return err
		}

		group := resp.AutoScalingGroups[0]

		instanceIds := []*string{}
		for _, instance := range group.Instances {
			instanceIds = append(instanceIds, instance.InstanceId)
		}

		_, err = fw.ec2.StartInstances(
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
	err = fw.stopInstances()

	if err == nil {
		err = fw.terminateAutoScaling()
	}
	if err == nil {
		err = fw.stopAutoScaling()
	}

	if err != nil {
		log.Printf("Error stopping: %v", err)
		return err
	}

	fw.ready = false
	fw.status = STOPPING
	fw.stopAt = fw.lastStopped
	return nil
}

// Stop EC2 instances
func (fw *Flywheel) stopInstances() error {
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
func (fw *Flywheel) stopAutoScaling() error {
	for _, groupName := range fw.config.AutoScaling.Stop {
		log.Printf("Stopping autoscaling group %s", groupName)

		resp, err := fw.autoscaling.DescribeAutoScalingGroups(
			&autoscaling.DescribeAutoScalingGroupsInput{
				AutoScalingGroupNames: []*string{&groupName},
			},
		)
		if err != nil {
			return err
		}

		group := resp.AutoScalingGroups[0]

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

		_, err = fw.ec2.StopInstances(
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
func (fw *Flywheel) terminateAutoScaling() error {
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
