package httpapi.authz

import future.keywords


# When provided with an admin role, will return an array
# with the admin role and an associated project role
get_valid_roles(s) := roleNames if {
	# build the project role name using projectID and admin role name
	# if projectID header was missing, input.project should be an empty string
	# and this function will build an invalid role
	roleParts := [input.project, s]
	projectRoleName := concat("_", roleParts)
	roleNames := [s, projectRoleName]
}

# alrt-r and <project-id>_alrt-r should allow to read api/v1/alerts and api/v1/alerts/definitions
allow_alrt_r if {
    allowed := get_valid_roles("alrt-r")
    some role in input.roles
	role in allowed
	input.method == "GET"
	input.path == ["api", "v1", "alerts"]
}

allow_alrt_r if {
    allowed := get_valid_roles("alrt-r")
    some role in input.roles
	role in allowed
	input.method == "GET"
	array.slice(input.path, 0, 4) == ["api", "v1", "alerts", "definitions"]
}

# alrt-rw and <project-id>_alrt-rw should allow to read and write to api/v1/alerts and api/v1/alerts/definitions
allow_alrt_rw if {
    allowed := get_valid_roles("alrt-rw")
    some role in input.roles
	role in allowed
	input.method == "GET"
	input.path == ["api", "v1", "alerts"]
}

allow_alrt_rw if {
    allowed := get_valid_roles("alrt-rw")
    some role in input.roles
	role in allowed
	input.method in ["GET", "PATCH"]
	array.slice(input.path, 0, 4) == ["api", "v1", "alerts", "definitions"]
}

# alrt-rx-rw should allow to read and write to api/v1/alerts/receivers
allow_alert_rx_rw if {
    some role in input.roles
	role == "alrt-rx-rw"
    input.method in ["GET", "PATCH"]
	array.slice(input.path, 0, 4) == ["api", "v1", "alerts", "receivers"]
}
