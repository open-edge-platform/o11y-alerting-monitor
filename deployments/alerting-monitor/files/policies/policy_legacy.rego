package httpapi.authz

import future.keywords

allow_alerts_read if {
	# alerts read role
	# allows access to edgenode/api/v1/alerts only
	some role in input.roles
	role == "alerts-read-role"
	input.method == "GET"
	input.path == ["edgenode", "api", "v1", "alerts"]
}

allow_alert_definitions_read if {
	# alerts read role
	# allows access to GET edgenode/api/v1/alerts/definitions/*
	some role in input.roles
	role == "alert-definitions-read-role"
	input.method == "GET"
	array.slice(input.path, 0, 5) == ["edgenode", "api", "v1", "alerts", "definitions"]
}

allow_alert_definitions_write if {
	# alerts write role
	# allows access to PATCH edgenode/api/v1/alerts/definitions/*
	some role in input.roles
	role == "alert-definitions-write-role"
	input.method == "PATCH"
	array.slice(input.path, 0, 5) == ["edgenode", "api", "v1", "alerts", "definitions"]
}

allow_alert_receivers_read if {
	# alerts receiver read role
	# allows access to GET edgenode/api/v1/alerts/receivers/*
	some role in input.roles
	role == "alert-receivers-read-role"
	input.method == "GET"
	array.slice(input.path, 0, 5) == ["edgenode", "api", "v1", "alerts", "receivers"]
}

allow_alert_receivers_write if {
	# alerts receiver write role
	# allows access to PATCH edgenode/api/v1/alerts/receivers/*
	some role in input.roles
	role == "alert-receivers-write-role"
	input.method == "PATCH"
	array.slice(input.path, 0, 5) == ["edgenode", "api", "v1", "alerts", "receivers"]
}
