-- Tabla de administradores
CREATE TABLE admins (
    id SERIAL PRIMARY KEY,
    user_name VARCHAR(150) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Tabla de tipos de vehículos
CREATE TABLE vehicle_types (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL -- auto, moto, suv
);

CREATE TABLE  reservation_times (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL -- hora, dia, semana, mes
);

CREATE TABLE vehicle_prices (
    id SERIAL PRIMARY KEY,
    vehicle_type_id INT NOT NULL REFERENCES vehicle_types(id),
    reservation_time_id INT NOT NULL REFERENCES reservation_times(id),
    price FLOAT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE vehicle_spaces (
    id SERIAL PRIMARY KEY,
    vehicle_type_id INT NOT NULL REFERENCES vehicle_types(id),
    spaces INT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Tabla de pagos
CREATE TABLE payment_method (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL -- on site or online
);

-- Tabla de reservas
CREATE TABLE reservations (
    id SERIAL PRIMARY KEY,
    code VARCHAR(10) UNIQUE NOT NULL,
    user_name VARCHAR(100) NOT NULL,
    user_email VARCHAR(150) NOT NULL,
    user_phone VARCHAR(20) NOT NULL,
    vehicle_type_id INT NOT NULL REFERENCES vehicle_types(id),
    payment_method_id INT NOT NULL REFERENCES payment_method(id),
    vehicle_plate VARCHAR(20) NOT NULL,
    vehicle_model VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'active',
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
    payment_status VARCHAR(50),
    stripe_session_id VARCHAR(255),
    stripe_payment_intent_id VARCHAR(255),
    language VARCHAR(20),
    total_price FLOAT
    deposit_payment FLOAT
);

INSERT INTO vehicle_types (name) VALUES ('car'), ('motorcycle'), ('suv');

INSERT INTO reservation_times (name) VALUES ('hour'), ('daily');

INSERT INTO vehicle_prices (vehicle_type_id, reservation_time_id, price)
VALUES 
    (1, 1, 4),  -- car hour
    (1, 2, 10), -- car daily
    (2, 1, 2),  -- motorcycle hour
    (2, 2, 8),  -- motorcycle daily
    (3, 1, 6),  -- suv hour
    (3, 2, 12); -- suv daily

INSERT INTO vehicle_spaces (vehicle_type_id, spaces)
VALUES 
    (1, 20),  -- car
    (2, 20),  -- motorcycle
    (3, 10);  -- suv

INSERT INTO payment_method (name)
VALUES 
    ('onsite'),
    ('online');
