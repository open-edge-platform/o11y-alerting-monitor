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

allow_alerts_read if {
	# alerts read role
	# allows access to api/v1/alerts only
	authorizedRoles := get_valid_roles("alerts-read-role")
	some role in input.roles
	role in authorizedRoles
	input.method == "GET"
	input.path == ["api", "v1", "alerts"]
}

allow_alert_definitions_read if {
	# alerts read role
	# allows access to GET api/v1/alerts/definitions/*
	authorizedRoles := get_valid_roles("alert-definitions-read-role")
	some role in input.roles
	role in authorizedRoles
	input.method == "GET"
	array.slice(input.path, 0, 4) == ["api", "v1", "alerts", "definitions"]
}

allow_alert_definitions_write if {
	# alerts write role
	# allows access to PATCH api/v1/alerts/definitions/*
	authorizedRoles := get_valid_roles("alert-definitions-write-role")
	some role in input.roles
	role in authorizedRoles
	input.method == "PATCH"
	array.slice(input.path, 0, 4) == ["api", "v1", "alerts", "definitions"]
}

allow_alert_receivers_read if {
	# alerts receiver read role
	# allows access to GET api/v1/alerts/receivers/*
	some role in input.roles
	role == "alert-receivers-read-role"
	input.method == "GET"
	array.slice(input.path, 0, 4) == ["api", "v1", "alerts", "receivers"]
}

allow_alert_receivers_write if {
	# alerts receiver write role
	# allows access to PATCH api/v1/alerts/receivers/*
	some role in input.roles
	role == "alert-receivers-write-role"
	input.method == "PATCH"
	array.slice(input.path, 0, 4) == ["api", "v1", "alerts", "receivers"]
}
