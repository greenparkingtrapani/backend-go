-- Green Parking: new vehicle sizes and e-bike parking (idempotent).
-- Existing 'car' is retained for historical reservations; the UI labels it 'Utilitaria'.
INSERT INTO vehicle_types (name)
SELECT v.name FROM (VALUES ('sedan'), ('suv'), ('electric_bicycle')) v(name)
WHERE NOT EXISTS (SELECT 1 FROM vehicle_types vt WHERE vt.name = v.name);

-- Set hourly rates as explicitly requested.
INSERT INTO vehicle_prices(vehicle_type_id,reservation_time_id,price)
SELECT vt.id, rt.id, CASE vt.name
 WHEN 'car' THEN 2.00 WHEN 'sedan' THEN 2.00 WHEN 'suv' THEN 3.00
 WHEN 'motorcycle' THEN 1.00 WHEN 'electric_bicycle' THEN 1.00 END
FROM vehicle_types vt CROSS JOIN reservation_times rt
WHERE rt.name = 'hour' AND vt.name IN ('car','sedan','suv','motorcycle','electric_bicycle')
ON CONFLICT(vehicle_type_id,reservation_time_id) DO UPDATE SET price=EXCLUDED.price;

-- Until a dedicated daily schedule is provided, inherit the existing daily
-- tariff of car/motorcycle. This never invents an unapproved daily charge.
INSERT INTO vehicle_prices(vehicle_type_id,reservation_time_id,price)
SELECT dest.id, rt.id, sourceprice.price
FROM (VALUES ('sedan','car'),('suv','car'),('electric_bicycle','motorcycle')) mapping(destination,source)
JOIN vehicle_types dest ON dest.name=mapping.destination
JOIN vehicle_types src ON src.name=mapping.source
JOIN reservation_times rt ON rt.name='daily'
JOIN vehicle_prices sourceprice ON sourceprice.vehicle_type_id=src.id AND sourceprice.reservation_time_id=rt.id
ON CONFLICT(vehicle_type_id,reservation_time_id) DO NOTHING;

-- All car sizes share the existing car capacity. Bikes share the existing
-- two-wheel pool. Capacity aggregation uses MAX, never SUM.
INSERT INTO vehicle_spaces(vehicle_type_id,spaces)
SELECT destination.id, existing.spaces
FROM (VALUES ('sedan','car'),('suv','car'),('electric_bicycle','motorcycle')) mapping(destination,source)
JOIN vehicle_types destination ON destination.name=mapping.destination
JOIN vehicle_types src ON src.name=mapping.source
JOIN vehicle_spaces existing ON existing.vehicle_type_id=src.id
WHERE NOT EXISTS(SELECT 1 FROM vehicle_spaces vs WHERE vs.vehicle_type_id=destination.id);
