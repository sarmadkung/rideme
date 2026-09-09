package merchant

import (
	"context"
	"fmt"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// Outlet is one of a merchant's stores (document 067).
//
// Not named Store, which in this package is the repository. The table is
// `stores`; this is the thing a customer picks before they fill a cart.
type Outlet struct {
	ID         string
	MerchantID string
	// MerchantName is what a customer recognises — they are choosing Al-Fatah,
	// not branch 4f3c.
	MerchantName string
	Name         string
	Address      string
	Status       string
	Lat          float64
	Lon          float64
	// DistanceM is metres from where the customer asked, so a client can sort
	// and label without recomputing what the database already knows.
	DistanceM float64
	// Open is the store's own hours applied to the moment of asking, not its
	// status: a store can be OPEN as a business and shut at 3am.
	Open bool
}

// Product is a catalogue line as a customer sees it (document 068).
type Product struct {
	ID          string
	Name        string
	Description string
	Price       money.Amount
	// Available is inventory at this store, not the product's own status: the
	// same product is on the shelf in Gulberg and not in DHA (document 069).
	Available bool
	Variants  []Variant
}

// Variant is a size or option, carrying its price difference rather than a
// price (document 068).
type Variant struct {
	ID    string
	Name  string
	Delta money.Amount
	// Available is this variant's inventory at the store being browsed.
	Available bool
}

// MaxOutlets and MaxProducts bound what one call returns. A customer scrolls a
// neighbourhood and a shop, not a country and a warehouse.
const (
	MaxOutlets  = 50
	MaxProducts = 200
)

// OutletsNear lists open-for-business stores within radius of a point.
//
// Ordered by distance because that is the only order a customer wants, and
// computed in the database because it already holds a GIST index over exactly
// this column.
func (s *Store) OutletsNear(ctx context.Context, lat, lon, radiusM float64, at time.Time,
	limit int) ([]Outlet, error) {
	if limit <= 0 || limit > MaxOutlets {
		limit = MaxOutlets
	}
	rows, err := s.pool.Query(ctx,
		`SELECT st.id::text, st.merchant_id::text, m.name, st.name,
		        COALESCE(st.address, ''), st.status,
		        COALESCE(ST_Y(st.location::geometry), 0), COALESCE(ST_X(st.location::geometry), 0),
		        ST_Distance(st.location,
		                    ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography) AS distance_m
		   FROM stores st
		   JOIN merchants m ON m.id = st.merchant_id
		  WHERE st.status = 'OPEN'
		    AND m.status = 'ACTIVE'
		    AND st.location IS NOT NULL
		    AND ST_DWithin(st.location, ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography, $3)
		  ORDER BY distance_m
		  LIMIT $4`, lat, lon, radiusM, limit)
	if err != nil {
		return nil, fmt.Errorf("list outlets: %w", err)
	}
	defer rows.Close()

	var out []Outlet
	ids := make([]string, 0, limit)
	for rows.Next() {
		var o Outlet
		if err := rows.Scan(&o.ID, &o.MerchantID, &o.MerchantName, &o.Name,
			&o.Address, &o.Status, &o.Lat, &o.Lon, &o.DistanceM); err != nil {
			return nil, fmt.Errorf("scan outlet: %w", err)
		}
		out = append(out, o)
		ids = append(ids, o.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list outlets: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}

	// One query for every listed store's hours rather than one per store: a
	// list of thirty shops must not be thirty-one round trips.
	hours, err := s.hoursFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Open = StoreOpenAt(hours[out[i].ID], at)
	}
	return out, nil
}

// OutletByID loads one store, for resolving which merchant a cart belongs to.
func (s *Store) OutletByID(ctx context.Context, id string) (Outlet, error) {
	var o Outlet
	err := s.pool.QueryRow(ctx,
		`SELECT st.id::text, st.merchant_id::text, m.name, st.name,
		        COALESCE(st.address, ''), st.status,
		        COALESCE(ST_Y(st.location::geometry), 0), COALESCE(ST_X(st.location::geometry), 0)
		   FROM stores st
		   JOIN merchants m ON m.id = st.merchant_id
		  WHERE st.id = $1 AND m.status = 'ACTIVE'`, id).
		Scan(&o.ID, &o.MerchantID, &o.MerchantName, &o.Name, &o.Address, &o.Status, &o.Lat, &o.Lon)
	if err != nil {
		return Outlet{}, ErrNotFound
	}
	return o, nil
}

func (s *Store) hoursFor(ctx context.Context, storeIDs []string) (map[string][]Hours, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT store_id::text, weekday,
		        EXTRACT(EPOCH FROM opens_at)::integer, EXTRACT(EPOCH FROM closes_at)::integer
		   FROM store_hours WHERE store_id = ANY($1::uuid[])`, storeIDs)
	if err != nil {
		return nil, fmt.Errorf("load store hours: %w", err)
	}
	defer rows.Close()

	out := map[string][]Hours{}
	for rows.Next() {
		var storeID string
		var window Hours
		if err := rows.Scan(&storeID, &window.Weekday, &window.OpensAt, &window.ClosesAt); err != nil {
			return nil, fmt.Errorf("scan store hours: %w", err)
		}
		out[storeID] = append(out[storeID], window)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load store hours: %w", err)
	}
	return out, nil
}

// ProductsAt lists what a store actually has, with its variants.
//
// Availability comes from the inventory row for this store, and a product with
// no inventory row there is not available: Reserve refuses it, so offering it
// would be a cart that fails at checkout for no visible reason (document 069).
func (s *Store) ProductsAt(ctx context.Context, storeID string, limit int) ([]Product, error) {
	if limit <= 0 || limit > MaxProducts {
		limit = MaxProducts
	}
	rows, err := s.pool.Query(ctx,
		`SELECT p.id::text, p.name, COALESCE(p.description, ''), p.price_minor, p.currency,
		        COALESCE(inv.available AND (inv.quantity IS NULL
		                 OR inv.reserved_quantity < inv.quantity), false)
		   FROM products p
		   JOIN stores st ON st.id = $1 AND st.merchant_id = p.merchant_id
		   LEFT JOIN inventory inv
		          ON inv.store_id = st.id AND inv.product_id = p.id AND inv.variant_id IS NULL
		  WHERE p.status = 'ACTIVE'
		  ORDER BY p.name
		  LIMIT $2`, storeID, limit)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	var out []Product
	ids := make([]string, 0, limit)
	for rows.Next() {
		var p Product
		var priceMinor int64
		var currency money.Currency
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &priceMinor, &currency,
			&p.Available); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		price, err := money.New(priceMinor, currency)
		if err != nil {
			return nil, err
		}
		p.Price = price
		out = append(out, p)
		ids = append(ids, p.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}

	variants, err := s.variantsFor(ctx, storeID, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Variants = variants[out[i].ID]
	}
	return out, nil
}

func (s *Store) variantsFor(ctx context.Context, storeID string,
	productIDs []string) (map[string][]Variant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT v.product_id::text, v.id::text, v.name, v.price_delta_minor, p.currency,
		        COALESCE(inv.available AND (inv.quantity IS NULL
		                 OR inv.reserved_quantity < inv.quantity), false)
		   FROM product_variants v
		   JOIN products p ON p.id = v.product_id
		   LEFT JOIN inventory inv
		          ON inv.store_id = $1 AND inv.product_id = v.product_id AND inv.variant_id = v.id
		  WHERE v.product_id = ANY($2::uuid[]) AND v.status = 'ACTIVE'
		  ORDER BY v.name`, storeID, productIDs)
	if err != nil {
		return nil, fmt.Errorf("load variants: %w", err)
	}
	defer rows.Close()

	out := map[string][]Variant{}
	for rows.Next() {
		var productID string
		var v Variant
		var deltaMinor int64
		var currency money.Currency
		if err := rows.Scan(&productID, &v.ID, &v.Name, &deltaMinor, &currency,
			&v.Available); err != nil {
			return nil, fmt.Errorf("scan variant: %w", err)
		}
		delta, err := money.New(deltaMinor, currency)
		if err != nil {
			return nil, err
		}
		v.Delta = delta
		out[productID] = append(out[productID], v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load variants: %w", err)
	}
	return out, nil
}

// OrdersOf lists a customer's own orders, newest first.
func (s *Store) OrdersOf(ctx context.Context, customerUserID string, limit int) ([]Order, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+orderColumns+`
		   FROM orders
		  WHERE customer_user_id = $1 AND status <> 'CART'
		  ORDER BY created_at DESC
		  LIMIT $2`, customerUserID, limit)
	if err != nil {
		return nil, fmt.Errorf("list customer orders: %w", err)
	}
	defer rows.Close()

	var out []Order
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan customer order: %w", err)
		}
		out = append(out, order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list customer orders: %w", err)
	}
	return out, nil
}
