package githubapi

import (
	"context"
)

// GetCustomPropertyValue returns the value of a custom property of a repo, or "" if it is not set or not a single value.
func (client *Client) GetCustomPropertyValue(org string, repo string, property string) (string, error) {
	ctx := context.Background()
	values, resp, err := client.gh.Repositories.GetAllCustomPropertyValues(ctx, org, repo)
	client.observe(resp, err)
	if err != nil {
		return "", err
	}

	for _, value := range values {
		if value.PropertyName == property {
			text, _ := value.Value.(string)
			return text, nil
		}
	}

	return "", nil
}
