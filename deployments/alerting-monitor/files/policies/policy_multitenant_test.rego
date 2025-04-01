package httpapi.authz

import future.keywords

# paths
alerts_path := ["api", "v1", "alerts"]
alerts_definitions_path := ["api", "v1", "alerts", "definitions"]
alerts_definitions_uuid_path := ["api", "v1", "alerts", "definitions", "some-uuid-here"]
alerts_definitions_uuid_template_path := ["api", "v1", "alerts", "definitions", "some-uuid-here", "template"]
alerts_receivers_path := ["api", "v1", "alerts", "receivers"]
alerts_receivers_uuid_path := ["api", "v1", "alerts", "receivers", "some-uuid-here"]
alerts_receivers_uuid_template_path := ["api", "v1", "alerts", "receivers", "some-uuid-here", "template"]
# NOTE: current opa policies do not enforce a valid UUID structure, hence the "some-uuid-here"

all_get_paths := [alerts_path, alerts_definitions_path, alerts_definitions_uuid_path, alerts_definitions_uuid_template_path, alerts_receivers_path, alerts_receivers_uuid_path, alerts_receivers_uuid_template_path]
all_patch_paths := [alerts_definitions_uuid_path, alerts_definitions_uuid_template_path, alerts_receivers_path, alerts_receivers_uuid_path, alerts_receivers_uuid_template_path]

# roles
alerts_r := ["11111111-1111-1111-1111-111111111111_alerts-read-role"]
alert_definitions_r := ["11111111-1111-1111-1111-111111111111_alert-definitions-read-role"]
alert_definitions_w := ["11111111-1111-1111-1111-111111111111_alert-definitions-write-role"]
alerts_admin_r := ["alerts-read-role"]
alert_admin_definitions_r := ["alert-definitions-read-role"]
alert_admin_definitions_w := ["alert-definitions-write-role"]
alert_admin_receivers_r := ["alert-receivers-read-role"]
alert_admin_receivers_w := ["alert-receivers-write-role"]
unauthorized_role := ["unauthorized-role-example"]

test_get_valid_roles if {
    alerts_admin_r[0] in get_valid_roles(alerts_admin_r[0]) with input as {"path": alerts_path, "project": "11111111-1111-1111-1111-111111111111"}
    alerts_r[0] in get_valid_roles(alerts_admin_r[0]) with input as {"path": alerts_path, "project": "11111111-1111-1111-1111-111111111111"}
}

test_alerts_endpoint if {
    allow_alerts_read with input as {"roles":alerts_r, "method":"GET", "path":alerts_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alerts_read with input as {"roles":alerts_admin_r, "method":"GET", "path":alerts_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_read with input as {"roles":alert_definitions_r, "method":"GET", "path":alerts_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_write with input as {"roles":alert_definitions_w, "method":"GET", "path":alerts_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_read with input as {"roles":alert_admin_receivers_r, "method":"GET", "path":alerts_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_write with input as {"roles":alert_admin_receivers_w, "method":"GET", "path":alerts_path, "project": "11111111-1111-1111-1111-111111111111"}
}

test_alerts_definitions_get_endpoint if {
    # /edgenode/api/v1/alerts/definitions
    not allow_alerts_read with input as {"roles":alerts_r, "method":"GET", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_definitions_read with input as {"roles":alert_definitions_r, "method":"GET", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_definitions_read with input as {"roles":alert_admin_definitions_r, "method":"GET", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_write with input as {"roles":alert_definitions_w, "method":"GET", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_read with input as {"roles":alert_admin_receivers_r, "method":"GET", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_write with input as {"roles":alert_admin_receivers_w, "method":"GET", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/definitions/<uuid>
    not allow_alerts_read with input as {"roles":alerts_r, "method":"GET", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_definitions_read with input as {"roles":alert_definitions_r, "method":"GET", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_definitions_read with input as {"roles":alert_admin_definitions_r, "method":"GET", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_write with input as {"roles":alert_definitions_w, "method":"GET", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_read with input as {"roles":alert_admin_receivers_r, "method":"GET", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_write with input as {"roles":alert_admin_receivers_w, "method":"GET", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/definitions/<uuid>/template
    not allow_alerts_read with input as {"roles":alerts_r, "method":"GET", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_definitions_read with input as {"roles":alert_definitions_r, "method":"GET", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_definitions_read with input as {"roles":alert_admin_definitions_r, "method":"GET", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_write with input as {"roles":alert_definitions_w, "method":"GET", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_read with input as {"roles":alert_admin_receivers_r, "method":"GET", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_write with input as {"roles":alert_admin_receivers_w, "method":"GET", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
}

test_alerts_definitions_patch_endpoint if {
    # /edgenode/api/v1/alerts/definitions
    not allow_alerts_read with input as {"roles":alerts_r, "method":"PATCH", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_read with input as {"roles":alert_definitions_r, "method":"PATCH", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_definitions_write with input as {"roles":alert_definitions_w, "method":"PATCH", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_definitions_write with input as {"roles":alert_admin_definitions_w, "method":"PATCH", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_read with input as {"roles":alert_admin_receivers_r, "method":"PATCH", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_write with input as {"roles":alert_admin_receivers_w, "method":"PATCH", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/definitions/<uuid>
    not allow_alerts_read with input as {"roles":alerts_r, "method":"PATCH", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_read with input as {"roles":alert_definitions_r, "method":"PATCH", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_definitions_write with input as {"roles":alert_definitions_w, "method":"PATCH", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_definitions_write with input as {"roles":alert_admin_definitions_w, "method":"PATCH", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_read with input as {"roles":alert_admin_receivers_r, "method":"PATCH", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_write with input as {"roles":alert_admin_receivers_w, "method":"PATCH", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/definitions/<uuid>/template
    not allow_alerts_read with input as {"roles":alerts_r, "method":"PATCH", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_read with input as {"roles":alert_definitions_r, "method":"PATCH", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_definitions_write with input as {"roles":alert_definitions_w, "method":"PATCH", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_definitions_write with input as {"roles":alert_admin_definitions_w, "method":"PATCH", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_read with input as {"roles":alert_admin_receivers_r, "method":"PATCH", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_write with input as {"roles":alert_admin_receivers_w, "method":"PATCH", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
}

test_alerts_receivers_get_endpoint if {
    # /edgenode/api/v1/alerts/receivers
    not allow_alerts_read with input as {"roles":alerts_admin_r, "method":"GET", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_read with input as {"roles":alert_admin_definitions_r, "method":"GET", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_write with input as {"roles":alert_admin_definitions_w, "method":"GET", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_receivers_read with input as {"roles":alert_admin_receivers_r, "method":"GET", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_write with input as {"roles":alert_admin_receivers_w, "method":"GET", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/receivers/<uuid>
    not allow_alerts_read with input as {"roles":alerts_admin_r, "method":"GET", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_read with input as {"roles":alert_admin_definitions_r, "method":"GET", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_write with input as {"roles":alert_admin_definitions_w, "method":"GET", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_receivers_read with input as {"roles":alert_admin_receivers_r, "method":"GET", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_write with input as {"roles":alert_admin_receivers_w, "method":"GET", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/receivers/<uuid>/template
    not allow_alerts_read with input as {"roles":alerts_admin_r, "method":"GET", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_read with input as {"roles":alert_admin_definitions_r, "method":"GET", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_write with input as {"roles":alert_admin_definitions_w, "method":"GET", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    #allow_alert_receivers_read with input as {"roles":alert_admin_receivers_r, "method":"GET", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_write with input as {"roles":alert_admin_receivers_w, "method":"GET", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
}

test_alerts_receivers_patch_endpoint if {
    # /edgenode/api/v1/alerts/receivers
    not allow_alerts_read with input as {"roles":alerts_admin_r, "method":"PATCH", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_read with input as {"roles":alert_admin_definitions_r, "method":"PATCH", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_write with input as {"roles":alert_admin_definitions_w, "method":"PATCH", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_read with input as {"roles":alert_admin_receivers_r, "method":"PATCH", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_receivers_write with input as {"roles":alert_admin_receivers_w, "method":"PATCH", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/receivers/<uuid>
    not allow_alerts_read with input as {"roles":alerts_admin_r, "method":"PATCH", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_read with input as {"roles":alert_admin_definitions_r, "method":"PATCH", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_write with input as {"roles":alert_admin_definitions_w, "method":"PATCH", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_read with input as {"roles":alert_admin_receivers_r, "method":"PATCH", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_receivers_write with input as {"roles":alert_admin_receivers_w, "method":"PATCH", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/receivers/<uuid>/template
    not allow_alerts_read with input as {"roles":alerts_admin_r, "method":"PATCH", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_read with input as {"roles":alert_admin_definitions_r, "method":"PATCH", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_write with input as {"roles":alert_admin_definitions_w, "method":"PATCH", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_read with input as {"roles":alert_admin_receivers_r, "method":"PATCH", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_receivers_write with input as {"roles":alert_admin_receivers_w, "method":"PATCH", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
}

# disallow all policies for an unauthorized role
test_unauthorized_alerts_read if {
    some path in all_get_paths
    not allow_alerts_read with input as {"roles":unauthorized_role, "method":"GET", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_read with input as {"roles":unauthorized_role, "method":"GET", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_write with input as {"roles":unauthorized_role, "method":"GET", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_read with input as {"roles":unauthorized_role, "method":"GET", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_write with input as {"roles":unauthorized_role, "method":"GET", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
}

test_unauthorized_alerts_patch if {
    some path in all_patch_paths
    not allow_alerts_read with input as {"roles":unauthorized_role, "method":"PATCH", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_read with input as {"roles":unauthorized_role, "method":"PATCH", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_definitions_write with input as {"roles":unauthorized_role, "method":"PATCH", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_read with input as {"roles":unauthorized_role, "method":"PATCH", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_receivers_write with input as {"roles":unauthorized_role, "method":"PATCH", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
}
