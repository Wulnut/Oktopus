/*
Provide answers to nats request-reply messages, executing queries to the database
*/
package reqs

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/OktopUSP/oktopus/backend/services/mtp/adapter/internal/db"
	local "github.com/OktopUSP/oktopus/backend/services/mtp/adapter/internal/nats"
	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type msgAnswer struct {
	Code int
	Msg  any
}

// extractTenantSlug extracts the tenant slug from a NATS subject.
// Subject format: adapter.usp.v1.<tenant_slug>.devices.<action>
// The tenant slug is at index 3.
func extractTenantSlug(subject string) string {
	parts := strings.Split(subject, ".")
	if len(parts) >= 4 {
		return parts[3]
	}
	return ""
}

func StartRequestsListener(ctx context.Context, nc *nats.Conn, db db.Database) {
	log.Println("Listening for nats requests")

	nc.QueueSubscribe(local.ADAPTER_SUBJECT+"*.device", local.ADAPTER_QUEUE, func(msg *nats.Msg) {
		tenantSlug := extractTenantSlug(msg.Subject)
		subject := strings.Split(msg.Subject, ".")
		device := subject[len(subject)-2]

		deviceInfo, err := db.RetrieveDevice(device, tenantSlug)
		if deviceInfo.SN != "" {
			respondMsg(msg.Respond, 200, deviceInfo)
		} else {
			if err != nil {
				if err == mongo.ErrNoDocuments {
					respondMsg(msg.Respond, 404, "Device not found")
				} else {
					respondMsg(msg.Respond, 500, err.Error())
				}
			}
		}
	})

	nc.QueueSubscribe(local.ADAPTER_SUBJECT+"devices.count", local.ADAPTER_QUEUE, func(msg *nats.Msg) {
		tenantSlug := extractTenantSlug(msg.Subject)
		count, err := db.RetrieveDevicesCount(tenantSlug)
		if err != nil {
			respondMsg(msg.Respond, 500, err.Error())
		}
		respondMsg(msg.Respond, 200, count)
	})

	nc.QueueSubscribe(local.ADAPTER_SUBJECT+"devices.retrieve", local.ADAPTER_QUEUE, func(msg *nats.Msg) {
		tenantSlug := extractTenantSlug(msg.Subject)

		var criteria map[string]interface{}

		err := json.Unmarshal(msg.Data, &criteria)
		if err != nil {
			respondMsg(msg.Respond, 500, err.Error())
		}

		//log.Println(criteria)
		propertiesFilter := bson.D{
			{Key: "tenantid", Value: tenantSlug},
		}

		vendorFilter := criteria["vendor"]
		if vendorFilter != nil {
			log.Println("Vendor filter", vendorFilter)
			propertiesFilter = append(propertiesFilter, bson.E{Key: "vendor", Value: vendorFilter})
		}

		versionFilter := criteria["version"]
		if versionFilter != nil {
			log.Println("Version filter", versionFilter)
			propertiesFilter = append(propertiesFilter, bson.E{Key: "version", Value: versionFilter})
		}

		typeFilter := criteria["productClass"]
		if typeFilter != nil {
			log.Println("Type filter", typeFilter)
			propertiesFilter = append(propertiesFilter, bson.E{Key: "productclass", Value: typeFilter})
		}

		aliasFilter := criteria["alias"]
		if aliasFilter != nil {
			log.Println("Type filter", aliasFilter)
			propertiesFilter = append(propertiesFilter, bson.E{Key: "alias", Value: aliasFilter})
		}

		modelFilter := criteria["model"]
		if modelFilter != nil {
			log.Println("Model filter", modelFilter)
			propertiesFilter = append(propertiesFilter, bson.E{Key: "model", Value: modelFilter})
		}

		statusFilter := criteria["status"]
		if statusFilter != nil {
			log.Println("Status filter", statusFilter)
			propertiesFilter = append(propertiesFilter, bson.E{Key: "status", Value: statusFilter})
		}

		filter := bson.A{
			bson.D{
				{"$match",
					propertiesFilter,
				},
			},
			bson.D{
				{"$facet",
					bson.D{
						{"totalCount",
							bson.A{
								bson.D{{"$count", "count"}},
							},
						},
						{"documents",
							bson.A{
								bson.D{{"$sort", bson.D{{"status", criteria["status_order"]}}}},
								bson.D{{"$skip", criteria["skip"]}},
								bson.D{{"$limit", criteria["limit"]}},
							},
						},
					},
				},
			},
			bson.D{
				{"$project",
					bson.D{
						{"totalCount",
							bson.D{
								{"$arrayElemAt",
									bson.A{
										"$totalCount.count",
										0,
									},
								},
							},
						},
						{"documents", 1},
					},
				},
			},
		}

		devicesList, err := db.RetrieveDevices(filter)
		if err != nil {
			respondMsg(msg.Respond, 500, err.Error())
		}
		respondMsg(msg.Respond, 200, &devicesList)
	})

	nc.QueueSubscribe(local.ADAPTER_SUBJECT+"devices.delete", local.ADAPTER_QUEUE, func(msg *nats.Msg) {
		tenantSlug := extractTenantSlug(msg.Subject)

		// Check if this is a "delete all" request (used during tenant deletion)
		var deleteAll struct {
			All bool `json:"all"`
		}
		if err := json.Unmarshal(msg.Data, &deleteAll); err == nil && deleteAll.All {
			deletedCount, err := db.DeleteDevices(bson.D{}, tenantSlug)
			if err != nil {
				respondMsg(msg.Respond, 500, err.Error())
				return
			}
			respondMsg(msg.Respond, 200, deletedCount)
			return
		}

		var serialNumbersList []string

		err := json.Unmarshal(msg.Data, &serialNumbersList)
		if err != nil {
			respondMsg(msg.Respond, 500, err.Error())
			return
		}

		var criteria bson.A

		for _, sn := range serialNumbersList {
			criteria = append(criteria, bson.D{{"sn", sn}})
		}

		filter := bson.D{
			{"$or", criteria},
		}

		deletedCount, err := db.DeleteDevices(filter, tenantSlug)
		if err != nil {
			respondMsg(msg.Respond, 500, err.Error())
			return
		}
		respondMsg(msg.Respond, 200, deletedCount)
	})

	nc.QueueSubscribe(local.ADAPTER_SUBJECT+"devices.filterOptions", local.ADAPTER_QUEUE, func(msg *nats.Msg) {
		tenantSlug := extractTenantSlug(msg.Subject)
		result, err := db.RetrieveDeviceFilterOptions(tenantSlug)
		if err != nil {
			respondMsg(msg.Respond, 500, err.Error())
		}
		respondMsg(msg.Respond, 200, result)
	})

	nc.QueueSubscribe(local.ADAPTER_SUBJECT+"devices.class", local.ADAPTER_QUEUE, func(msg *nats.Msg) {
		tenantSlug := extractTenantSlug(msg.Subject)
		productClassCount, err := db.RetrieveProductsClassInfo(tenantSlug)
		if err != nil {
			respondMsg(msg.Respond, 500, err.Error())
		}
		respondMsg(msg.Respond, 200, productClassCount)
	})

	nc.QueueSubscribe(local.ADAPTER_SUBJECT+"devices.vendors", local.ADAPTER_QUEUE, func(msg *nats.Msg) {
		tenantSlug := extractTenantSlug(msg.Subject)
		productClassCount, err := db.RetrieveVendorsInfo(tenantSlug)
		if err != nil {
			respondMsg(msg.Respond, 500, err.Error())
		}
		respondMsg(msg.Respond, 200, productClassCount)
	})

	nc.QueueSubscribe(local.ADAPTER_SUBJECT+"devices.status", local.ADAPTER_QUEUE, func(msg *nats.Msg) {
		tenantSlug := extractTenantSlug(msg.Subject)
		productClassCount, err := db.RetrieveStatusInfo(tenantSlug)
		if err != nil {
			respondMsg(msg.Respond, 500, err.Error())
		}
		respondMsg(msg.Respond, 200, productClassCount)
	})

	nc.QueueSubscribe(local.ADAPTER_SUBJECT+"*.device.alias", local.ADAPTER_QUEUE, func(msg *nats.Msg) {
		tenantSlug := extractTenantSlug(msg.Subject)
		subject := strings.Split(msg.Subject, ".")
		device := subject[len(subject)-3]

		err := db.SetDeviceAlias(device, string(msg.Data), tenantSlug)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				respondMsg(msg.Respond, 404, "Device not found")
			} else {
				respondMsg(msg.Respond, 500, err.Error())
			}
			return
		}
		respondMsg(msg.Respond, 200, "Alias updated")
	})
}

func respondMsg(respond func(data []byte) error, code int, msgData any) {

	msg, err := json.Marshal(msgAnswer{
		Code: code,
		Msg:  msgData,
	})
	if err != nil {
		log.Printf("Failed to marshal message: %q", err)
		respond([]byte(err.Error()))
		return
	}

	respond([]byte(msg))
}
