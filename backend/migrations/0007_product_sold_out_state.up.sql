UPDATE products SET status = 'OFF_SHELF' WHERE status = 'CLOSED';
UPDATE products SET status = 'OFF_SHELF' WHERE status = 'SOLD' AND stock > 0;
