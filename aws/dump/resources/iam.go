package resources

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/fatih/structs"
)

var (
	IAMService = Service{
		Name:     "iam",
		IsGlobal: true,
		Reports: map[string]Report{
			"users-and-access-keys": IAMListUsersAndAccessKeys,
			"roles":                 IAMListRoles,
			"policies":              IAMListPolicies,
			"groups":                IAMListGroups,
			"instance-profiles":     IAMListInstanceProfiles,
		},
	}
)

func IAMListUsersAndAccessKeys(session *Session) *ReportResult {
	client := iam.New(session.Session, session.Config)
	accessKeys := []Resource{}
	arns := []*string{}
	result := &ReportResult{}
	result.Error = client.ListUsersPages(&iam.ListUsersInput{},
		func(page *iam.ListUsersOutput, lastPage bool) bool {
			for _, user := range page.Users {
				resource, err := NewResource(*user.Arn, user)
				if err != nil {
					result.Error = err
					return false
				}
				arns = append(arns, user.Arn)
				result.Resources = append(result.Resources, *resource)

				keysResult := IAMListAccessKeys(session, client, *user.UserName)
				if keysResult.Error != nil {
					result.Error = keysResult.Error
					return false
				}
				accessKeys = append(accessKeys, keysResult.Resources...)
			}

			return true
		})

	if result.Error != nil {
		return result
	}

	jobIds, err := GenerateServiceLastAccessedDetails(client, arns)
	if err != nil {
		result.Error = err
		return result
	}
	AttachServiceLastAccessedDetails(client, result, jobIds)

	result.Resources = append(result.Resources, accessKeys...)
	return result
}

func IAMListGroups(session *Session) *ReportResult {
	client := iam.New(session.Session, session.Config)
	arns := []*string{}
	result := &ReportResult{}
	result.Error = client.ListGroupsPages(&iam.ListGroupsInput{},
		func(page *iam.ListGroupsOutput, lastPage bool) bool {
			for _, group := range page.Groups {

				resource, err := NewResource(*group.Arn, group)
				if err != nil {
					result.Error = err
					return false
				}
				arns = append(arns, group.Arn)
				result.Resources = append(result.Resources, *resource)
			}

			return true
		})

	if result.Error != nil {
		return result
	}

	jobIds, err := GenerateServiceLastAccessedDetails(client, arns)
	if err != nil {
		result.Error = err
		return result
	}
	AttachServiceLastAccessedDetails(client, result, jobIds)

	return result
}

func IAMListRoles(session *Session) *ReportResult {
	client := iam.New(session.Session, session.Config)
	arns := []*string{}
	result := &ReportResult{}
	result.Error = client.ListRolesPages(&iam.ListRolesInput{},
		func(page *iam.ListRolesOutput, lastPage bool) bool {
			for _, role := range page.Roles {
				resource, err := NewResource(*role.Arn, role)
				if err != nil {
					result.Error = err
					return false
				}

				document, err := DecodeInlinePolicyDocument(*resource.Metadata["AssumeRolePolicyDocument"].(*string))
				if err != nil {
					result.Error = err
					return false
				}
				resource.Metadata["AssumeRolePolicyDocument"] = document

				resource.ID = *role.RoleId
				arns = append(arns, role.Arn)
				result.Resources = append(result.Resources, *resource)
			}

			return true
		})

	if result.Error != nil {
		return result
	}

	jobIds, err := GenerateServiceLastAccessedDetails(client, arns)
	if err != nil {
		result.Error = err
		return result
	}
	AttachServiceLastAccessedDetails(client, result, jobIds)

	return result
}

func IAMListPolicyVersions(session *Session, client *iam.IAM, policyArn string) *ReportResult {
	result := &ReportResult{}
	err := client.ListPolicyVersionsPages(&iam.ListPolicyVersionsInput{PolicyArn: aws.String(policyArn)},
		func(page *iam.ListPolicyVersionsOutput, lastPage bool) bool {
			for _, resource := range page.Versions {

				policyVersion, err := client.GetPolicyVersion(&iam.GetPolicyVersionInput{PolicyArn: aws.String(policyArn), VersionId: resource.VersionId})
				if err != nil {
					result.Error = err
					return false
				}

				document, err := DecodeInlinePolicyDocument(*policyVersion.PolicyVersion.Document)
				if err != nil {
					result.Error = err
					return false
				}

				metadata := structs.Map(policyVersion.PolicyVersion)
				metadata["Document"] = document

				arn := fmt.Sprintf("%s:%s", policyArn, *resource.VersionId)
				r := Resource{
					ID:        arn,
					ARN:       arn,
					AccountID: session.AccountID,
					Service:   "iam",
					Type:      "policy-version",
					Region:    *session.Config.Region,
					Metadata:  metadata,
				}
				result.Resources = append(result.Resources, r)
			}
			return true
		})

	if result.Error != nil {
		return result
	}

	result.Error = err
	return result
}

func IAMListPolicies(session *Session) *ReportResult {
	client := iam.New(session.Session, session.Config)
	arns := []*string{}
	result := &ReportResult{}
	result.Error = client.ListPoliciesPages(&iam.ListPoliciesInput{Scope: aws.String("Local")},
		func(page *iam.ListPoliciesOutput, lastPage bool) bool {
			for _, policy := range page.Policies {
				resource, err := NewResource(*policy.Arn, policy)
				if err != nil {
					result.Error = err
					return false
				}

				arns = append(arns, policy.Arn)

				policyVersions := IAMListPolicyVersions(session, client, *policy.Arn)
				if policyVersions.Error != nil {
					result.Error = policyVersions.Error
					return false
				}

				result.Resources = append(result.Resources, *resource)
				result.Resources = append(result.Resources, policyVersions.Resources...)
			}

			return true
		})

	if result.Error != nil {
		return result
	}

	jobIds, err := GenerateServiceLastAccessedDetails(client, arns)
	if err != nil {
		result.Error = err
		return result
	}
	AttachServiceLastAccessedDetails(client, result, jobIds)
	return result
}

func IAMListAccessKeys(session *Session, client *iam.IAM, username string) *ReportResult {
	result := &ReportResult{}
	result.Error = client.ListAccessKeysPages(&iam.ListAccessKeysInput{
		UserName: aws.String(username),
	},
		func(page *iam.ListAccessKeysOutput, lastPage bool) bool {
			for _, accessKey := range page.AccessKeyMetadata {
				resource := Resource{
					ID:        *accessKey.AccessKeyId,
					AccountID: session.AccountID,
					Service:   "iam",
					Type:      "access-key",
					Metadata:  structs.Map(accessKey),
				}

				lastUsed, err := client.GetAccessKeyLastUsed(&iam.GetAccessKeyLastUsedInput{AccessKeyId: accessKey.AccessKeyId})
				if err != nil {
					result.Error = err
					return false
				}
				resource.Metadata["AccessKeyLastUsed"] = structs.Map(lastUsed.AccessKeyLastUsed)
				resource.Metadata["LastUsed"] = lastUsed.AccessKeyLastUsed.LastUsedDate
				result.Resources = append(result.Resources, resource)
			}

			return true
		})

	return result
}

func GenerateServiceLastAccessedDetails(client *iam.IAM, arns []*string) ([]*string, error) {
	jobIds := []*string{}
	for _, arn := range arns {
		job, err := client.GenerateServiceLastAccessedDetails(&iam.GenerateServiceLastAccessedDetailsInput{
			Arn: arn,
		})
		if err != nil {
			return nil, err
		}
		jobIds = append(jobIds, job.JobId)
	}
	return jobIds, nil
}

func AttachServiceLastAccessedDetails(client *iam.IAM, result *ReportResult, jobIds []*string) {
	for i := 0; i < len(jobIds); {
		jobId := jobIds[i]
		lastUsed, err := client.GetServiceLastAccessedDetails(&iam.GetServiceLastAccessedDetailsInput{JobId: jobId})
		if err != nil {
			result.Error = err
			return
		}
		if *lastUsed.JobStatus == "IN_PROGRESS" {
			time.Sleep(1 * time.Second)
			continue
		}
		if *lastUsed.JobStatus == "COMPLETED" {
			result.Resources[i].Metadata["ServiceLastAccessed"] = lastUsed.ServicesLastAccessed
			var lastUsedAt *time.Time
			for _, serviceLastAccessed := range lastUsed.ServicesLastAccessed {
				if serviceLastAccessed.LastAuthenticated == nil {
					continue
				}
				if lastUsedAt == nil || serviceLastAccessed.LastAuthenticated.After(*lastUsedAt) {
					lastUsedAt = serviceLastAccessed.LastAuthenticated
				}
			}
			result.Resources[i].Metadata["LastUsed"] = lastUsedAt

		}
		i += 1
	}
}

func IAMListInstanceProfiles(session *Session) *ReportResult {

	client := iam.New(session.Session, session.Config)

	result := &ReportResult{}
	err := client.ListInstanceProfilesPages(&iam.ListInstanceProfilesInput{},
		func(page *iam.ListInstanceProfilesOutput, lastPage bool) bool {
			for _, instanceProfile := range page.InstanceProfiles {
				resource := Resource{
					ID:        *instanceProfile.InstanceProfileId,
					ARN:       *instanceProfile.Arn,
					AccountID: session.AccountID,
					Service:   "iam",
					Type:      "instance-profile",
					Region:    *session.Config.Region,
					Metadata:  structs.Map(instanceProfile),
				}

				roles := resource.Metadata["Roles"].([]interface{})
				for _, irole := range roles {
					role := irole.(map[string]interface{})
					document, err := DecodeInlinePolicyDocument(*role["AssumeRolePolicyDocument"].(*string))
					if err != nil {
						result.Error = err
						return false
					}
					role["AssumeRolePolicyDocument"] = document
				}

				result.Resources = append(result.Resources, resource)
			}

			return true
		})

	if result.Error != nil {
		return result
	}
	result.Error = err
	return result
}
